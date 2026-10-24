package clientupdates

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const schema = `CREATE TABLE IF NOT EXISTS client_releases (
 id bigserial PRIMARY KEY, product_id text NOT NULL DEFAULT 'fastaiapi-codexplus', version text NOT NULL,
 channel text NOT NULL DEFAULT 'stable', notes text NOT NULL DEFAULT '', status text NOT NULL DEFAULT 'draft',
 assets jsonb NOT NULL DEFAULT '[]', envelope jsonb, required_targets jsonb NOT NULL,
 created_by bigint NOT NULL, published_by bigint, created_at timestamptz NOT NULL DEFAULT now(), published_at timestamptz,
 UNIQUE(product_id,channel,version));
 CREATE TABLE IF NOT EXISTS client_release_channels(product_id text NOT NULL,channel text NOT NULL,release_id bigint REFERENCES client_releases(id),PRIMARY KEY(product_id,channel));
 CREATE TABLE IF NOT EXISTS client_release_audits(id bigserial PRIMARY KEY,release_id bigint NOT NULL,actor bigint NOT NULL,action text NOT NULL,reason text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now());`

type Release struct {
	ID        int64     `json:"id"`
	Version   string    `json:"version"`
	Channel   string    `json:"channel"`
	Notes     string    `json:"notes"`
	Status    string    `json:"status"`
	Assets    []Asset   `json:"assets"`
	Required  []string  `json:"required_targets"`
	Envelope  *Envelope `json:"envelope,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Service struct {
	DB      *sql.DB
	AdminDB *sql.DB
	Root    string
	Public  ed25519.PublicKey
	mu      sync.Mutex
}

func New(ctx context.Context, db, admin *sql.DB, root string, pub ed25519.PublicKey) (*Service, error) {
	if root == "" || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("更新目录或公钥未配置")
	}
	if e := os.MkdirAll(filepath.Join(root, "objects"), 0700); e != nil {
		return nil, e
	}
	if _, e := db.ExecContext(ctx, schema); e != nil {
		return nil, e
	}
	return &Service{DB: db, AdminDB: admin, Root: root, Public: pub}, nil
}
func (s *Service) IsAdmin(ctx context.Context, uid int64) bool {
	var ok bool
	if s.AdminDB == nil {
		return false
	}
	e := s.AdminDB.QueryRowContext(ctx, `SELECT role='admin' AND status='active' AND deleted_at IS NULL FROM users WHERE id=$1`, uid).Scan(&ok)
	return e == nil && ok
}

const columns = `id,version,channel,notes,status,assets,required_targets,envelope,created_at`

func scan(row interface{ Scan(...any) error }) (Release, error) {
	var r Release
	var assets, required, envelope []byte
	e := row.Scan(&r.ID, &r.Version, &r.Channel, &r.Notes, &r.Status, &assets, &required, &envelope, &r.CreatedAt)
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal(assets, &r.Assets); e != nil {
		return r, e
	}
	if e = json.Unmarshal(required, &r.Required); e != nil {
		return r, e
	}
	if len(envelope) > 0 && string(envelope) != "null" {
		r.Envelope = &Envelope{}
		e = json.Unmarshal(envelope, r.Envelope)
	}
	return r, e
}
func (s *Service) Get(ctx context.Context, id int64) (Release, error) {
	return scan(s.DB.QueryRowContext(ctx, `SELECT `+columns+` FROM client_releases WHERE id=$1`, id))
}
func (s *Service) List(ctx context.Context) ([]Release, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT `+columns+` FROM client_releases ORDER BY id DESC LIMIT 100`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Release{}
	for rows.Next() {
		r, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Service) Current(ctx context.Context) (Release, error) {
	return scan(s.DB.QueryRowContext(ctx, `SELECT `+strings.ReplaceAll(columns, ",", ",r.")+` FROM client_releases r JOIN client_release_channels c ON r.id=c.release_id WHERE c.product_id=$1 AND c.channel='stable' AND r.status='published'`, Product))
}
func (s *Service) Create(ctx context.Context, uid int64, version, notes string, required []string) (Release, error) {
	if !VersionValid(version) || len(notes) > 16000 || len(required) == 0 || len(required) > 3 {
		return Release{}, errors.New("版本或目标平台无效")
	}
	seen := map[string]bool{}
	for _, t := range required {
		if seen[t] || !(t == "windows/x64" || t == "macos/universal" || t == "macos/arm64" || t == "macos/x64") {
			return Release{}, errors.New("目标平台无效")
		}
		seen[t] = true
	}
	b, _ := json.Marshal(required)
	return scan(s.DB.QueryRowContext(ctx, `INSERT INTO client_releases(version,notes,required_targets,created_by) VALUES($1,$2,$3,$4) RETURNING `+columns, version, notes, b, uid))
}
func (s *Service) Upload(ctx context.Context, id, uid int64, platform, arch, name string, reader io.Reader) (Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.Get(ctx, id)
	if e != nil {
		return r, e
	}
	if r.Status != "draft" {
		return r, errors.New("仅草稿允许上传")
	}
	a := Asset{Platform: platform, Arch: arch, Name: name, Size: 1, SHA256: strings.Repeat("0", 64)}
	a.URL = AssetURL(r.Version, a.SHA256, name)
	if e = (Manifest{Schema: 1, Product: Product, Version: r.Version, Channel: "stable", Sequence: id, Assets: []Asset{a}}).Validate(); e != nil {
		return r, e
	}
	var random [16]byte
	if _, e = rand.Read(random[:]); e != nil {
		return r, e
	}
	tmp := filepath.Join(s.Root, hex.EncodeToString(random[:])+".part")
	f, e := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return r, e
	}
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, h), io.LimitReader(reader, (2<<30)+1))
	closeErr := f.Close()
	if e != nil {
		return r, e
	}
	if closeErr != nil {
		return r, closeErr
	}
	if n <= 0 || n > 2<<30 {
		return r, errors.New("安装包为空或超过 2 GiB")
	}
	a.Size = n
	a.SHA256 = hex.EncodeToString(h.Sum(nil))
	a.URL = AssetURL(r.Version, a.SHA256, a.Name)
	dir := filepath.Join(s.Root, "objects", a.SHA256)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return r, e
	}
	target := filepath.Join(dir, a.Name)
	// 内容寻址：相同目标只接受已存在的相同摘要，避免覆盖已发布文件。
	if _, e = os.Stat(target); os.IsNotExist(e) {
		e = os.Rename(tmp, target)
	} else if e == nil {
		e = s.verifyFile(a)
	}
	if e != nil {
		return r, e
	}
	next := []Asset{}
	for _, v := range r.Assets {
		if v.Platform != platform || v.Arch != arch {
			next = append(next, v)
		}
	}
	next = append(next, a)
	b, _ := json.Marshal(next)
	result, e := s.DB.ExecContext(ctx, `UPDATE client_releases SET assets=$1,envelope=NULL WHERE id=$2 AND status='draft'`, b, id)
	if e != nil {
		return r, e
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return r, errors.New("草稿状态已改变")
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO client_release_audits(release_id,actor,action) VALUES($1,$2,'upload')`, id, uid)
	if e != nil {
		return r, e
	}
	return s.Get(ctx, id)
}
func (s *Service) verifyFile(a Asset) error {
	f, e := os.Open(filepath.Join(s.Root, "objects", a.SHA256, a.Name))
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, f)
	if e != nil {
		return e
	}
	if n != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return errors.New("安装包大小或 SHA-256 校验失败")
	}
	return nil
}
func DraftManifest(r Release) Manifest {
	return Manifest{Schema: 1, Product: Product, Version: r.Version, Channel: r.Channel, Sequence: r.ID, PublishedAt: r.CreatedAt.UTC().Format(time.RFC3339), Notes: r.Notes, Assets: r.Assets}
}
func matches(r Release, m Manifest) error {
	if m.Version != r.Version || m.Sequence != r.ID || m.Notes != r.Notes || m.Channel != r.Channel || len(m.Assets) != len(r.Assets) {
		return errors.New("签名清单与当前草稿不一致")
	}
	wanted := map[string]bool{}
	for _, t := range r.Required {
		wanted[t] = false
	}
	for _, a := range m.Assets {
		target := a.Platform + "/" + a.Arch
		if _, ok := wanted[target]; !ok {
			return errors.New("安装包不在声明的平台范围内")
		}
		wanted[target] = true
		found := false
		for _, b := range r.Assets {
			if a == b {
				found = true
			}
		}
		if !found {
			return errors.New("安装包与签名清单不一致")
		}
	}
	for _, found := range wanted {
		if !found {
			return errors.New("必需平台的安装包未齐备")
		}
	}
	return nil
}
func (s *Service) Validate(ctx context.Context, id, uid int64, env Envelope) (Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.Get(ctx, id)
	if e != nil {
		return r, e
	}
	if r.Status != "draft" {
		return r, errors.New("只能校验草稿")
	}
	m, e := Verify(env, s.Public)
	if e != nil {
		return r, e
	}
	if e = matches(r, m); e != nil {
		return r, e
	}
	for _, a := range m.Assets {
		if e = s.verifyFile(a); e != nil {
			return r, e
		}
	}
	b, _ := json.Marshal(env)
	result, e := s.DB.ExecContext(ctx, `UPDATE client_releases SET envelope=$1,status='ready' WHERE id=$2 AND status='draft'`, b, id)
	if e != nil {
		return r, e
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return r, errors.New("版本状态已改变")
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO client_release_audits(release_id,actor,action) VALUES($1,$2,'validate')`, id, uid)
	if e != nil {
		return r, e
	}
	return s.Get(ctx, id)
}
func (s *Service) Publish(ctx context.Context, id, uid int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.Get(ctx, id)
	if e != nil {
		return e
	}
	if r.Status != "ready" || r.Envelope == nil {
		return errors.New("版本尚未通过签名校验")
	}
	m, e := Verify(*r.Envelope, s.Public)
	if e != nil {
		return e
	}
	if e = matches(r, m); e != nil {
		return e
	}
	for _, a := range m.Assets {
		if e = s.verifyFile(a); e != nil {
			return e
		}
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(167281041)`); e != nil {
		return e
	}
	var previous string
	var previousID int64
	e = tx.QueryRowContext(ctx, `SELECT version,id FROM client_releases WHERE product_id=$1 AND channel='stable' AND published_at IS NOT NULL ORDER BY id DESC LIMIT 1`, Product).Scan(&previous, &previousID)
	if e != nil && e != sql.ErrNoRows {
		return e
	}
	if e == nil && (!Newer(r.Version, previous) || id <= previousID) {
		return errors.New("发布版本和序号必须高于当前版本")
	}
	result, e := tx.ExecContext(ctx, `UPDATE client_releases SET status='published',published_by=$1,published_at=now() WHERE id=$2 AND status='ready'`, uid, id)
	if e != nil {
		return e
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("版本已被其他管理员修改")
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO client_release_channels(product_id,channel,release_id) VALUES($1,'stable',$2) ON CONFLICT(product_id,channel) DO UPDATE SET release_id=EXCLUDED.release_id`, Product, id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO client_release_audits(release_id,actor,action) VALUES($1,$2,'publish')`, id, uid); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Service) Withdraw(ctx context.Context, id, uid int64, reason string) error {
	if len(strings.TrimSpace(reason)) < 2 || len(reason) > 500 {
		return errors.New("请填写撤回原因")
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(167281041)`); e != nil {
		return e
	}
	result, e := tx.ExecContext(ctx, `UPDATE client_releases SET status='withdrawn' WHERE id=$1 AND status IN ('published','ready')`, id)
	if e != nil {
		return e
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("版本不可撤回")
	}
	if _, e = tx.ExecContext(ctx, `UPDATE client_release_channels SET release_id=NULL WHERE release_id=$1`, id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO client_release_audits(release_id,actor,action,reason) VALUES($1,$2,'withdraw',$3)`, id, uid, reason); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Service) Download(ctx context.Context, version, hash, name string) (string, error) {
	if !filenamePattern.MatchString(name) || !VersionValid(version) || len(hash) != 64 {
		return "", errors.New("无效下载路径")
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT `+columns+` FROM client_releases WHERE product_id=$1 AND version=$2 AND status='published'`, Product, version)
	if e != nil {
		return "", e
	}
	defer rows.Close()
	for rows.Next() {
		r, e := scan(rows)
		if e != nil {
			return "", e
		}
		for _, a := range r.Assets {
			if a.SHA256 == hash && a.Name == name {
				return filepath.Join(s.Root, "objects", hash, name), nil
			}
		}
	}
	return "", fmt.Errorf("安装包尚未发布或已撤回")
}
