package clientupdates

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
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
 CREATE TABLE IF NOT EXISTS client_release_audits(id bigserial PRIMARY KEY,release_id bigint NOT NULL,actor bigint NOT NULL,action text NOT NULL,reason text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now());
 ALTER TABLE client_releases ADD COLUMN IF NOT EXISTS release_options jsonb NOT NULL DEFAULT '{}';
 CREATE TABLE IF NOT EXISTS client_release_uploads (
  id text PRIMARY KEY, product_id text NOT NULL, release_id bigint NOT NULL REFERENCES client_releases(id), actor bigint NOT NULL,
  platform text NOT NULL, arch text NOT NULL, name text NOT NULL, total_bytes bigint NOT NULL, received_bytes bigint NOT NULL DEFAULT 0,
  complete boolean NOT NULL DEFAULT false, updated_at timestamptz NOT NULL DEFAULT now());`

type ReleaseOptions struct {
	Mandatory   bool   `json:"mandatory"`
	RuntimeHash string `json:"runtime_sha256,omitempty"`
}

type Release struct {
	ID          int64          `json:"id"`
	Product     string         `json:"product_id"`
	Options     ReleaseOptions `json:"options"`
	Version     string         `json:"version"`
	Channel     string         `json:"channel"`
	Notes       string         `json:"notes"`
	Status      string         `json:"status"`
	Assets      []Asset        `json:"assets"`
	Required    []string       `json:"required_targets"`
	Envelope    *Envelope      `json:"envelope,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	PublishedAt *time.Time     `json:"published_at,omitempty"`
}
type Service struct {
	DB         *sql.DB
	AdminDB    *sql.DB
	Root       string
	Public     ed25519.PublicKey
	Product    string
	SigningKey ed25519.PrivateKey `json:"-"`
	mu         sync.Mutex
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
	return &Service{DB: db, AdminDB: admin, Root: root, Public: pub, Product: Product}, nil
}

func (s *Service) product() string {
	if s.Product == "" {
		return Product
	}
	return s.Product
}
func (s *Service) IsAdmin(ctx context.Context, uid int64) bool {
	var ok bool
	if s.AdminDB == nil {
		return false
	}
	e := s.AdminDB.QueryRowContext(ctx, `SELECT role='admin' AND status='active' AND deleted_at IS NULL FROM users WHERE id=$1`, uid).Scan(&ok)
	return e == nil && ok
}

const columns = `id,version,channel,notes,status,assets,required_targets,envelope,created_at,product_id,release_options,published_at`

func scan(row interface{ Scan(...any) error }) (Release, error) {
	var r Release
	var assets, required, envelope, options []byte
	e := row.Scan(&r.ID, &r.Version, &r.Channel, &r.Notes, &r.Status, &assets, &required, &envelope, &r.CreatedAt, &r.Product, &options, &r.PublishedAt)
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
	if e == nil {
		e = json.Unmarshal(options, &r.Options)
	}
	return r, e
}
func (s *Service) Get(ctx context.Context, id int64) (Release, error) {
	return scan(s.DB.QueryRowContext(ctx, `SELECT `+columns+` FROM client_releases WHERE id=$1 AND product_id=$2`, id, s.product()))
}
func (s *Service) List(ctx context.Context) ([]Release, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT `+columns+` FROM client_releases WHERE product_id=$1 ORDER BY id DESC LIMIT 100`, s.product())
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
	return scan(s.DB.QueryRowContext(ctx, `SELECT r.`+strings.ReplaceAll(columns, ",", ",r.")+` FROM client_releases r JOIN client_release_channels c ON r.id=c.release_id WHERE c.product_id=$1 AND r.product_id=$1 AND c.channel='stable' AND r.status='published'`, s.product()))
}

func (s *Service) InstallerRelease(ctx context.Context) (Release, error) {
	return scan(s.DB.QueryRowContext(ctx, `SELECT `+columns+` FROM client_releases WHERE product_id=$1 AND status='published' AND EXISTS (SELECT 1 FROM jsonb_array_elements(assets) asset WHERE asset->>'platform' IN ('macos','windows')) ORDER BY id DESC LIMIT 1`, s.product()))
}

func (s *Service) EditOptions(ctx context.Context, id, uid int64, notes string, mandatory bool) (Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.product() != ManjuProduct || len(notes) > 16000 {
		return Release{}, errors.New("发布说明无效")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Release{}, err
	}
	defer tx.Rollback()
	r, err := s.lockRelease(ctx, tx, id)
	if err != nil {
		return r, err
	}
	if !s.canEdit(r) {
		return r, errors.New("只有草稿和笑笑漫剧已发布版本可以修改")
	}
	r.Notes = notes
	r.Options.Mandatory = mandatory
	r, err = s.saveChanges(ctx, tx, r, uid, "edit")
	if err != nil {
		return r, err
	}
	return r, tx.Commit()
}

func (s *Service) canEdit(r Release) bool {
	return r.Status == "draft" || (r.Status == "published" && s.product() == ManjuProduct)
}

func (s *Service) lockRelease(ctx context.Context, tx *sql.Tx, id int64) (Release, error) {
	return scan(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM client_releases WHERE id=$1 AND product_id=$2 FOR UPDATE`, id, s.product()))
}

func (s *Service) saveChanges(ctx context.Context, tx *sql.Tx, r Release, uid int64, action string) (Release, error) {
	var envelope []byte
	r.Envelope = nil
	if r.Status == "published" {
		signed, err := s.signRelease(r)
		if err != nil {
			return r, err
		}
		manifest, err := VerifyProduct(signed, s.Public, s.product())
		if err != nil {
			return r, err
		}
		if err = matches(r, manifest); err != nil {
			return r, err
		}
		envelope, _ = json.Marshal(signed)
		r.Envelope = &signed
	}
	options, _ := json.Marshal(r.Options)
	assets, _ := json.Marshal(r.Assets)
	_, err := tx.ExecContext(ctx, `UPDATE client_releases SET notes=$1,release_options=$2,assets=$3,envelope=$4 WHERE id=$5 AND product_id=$6`, r.Notes, options, assets, envelope, r.ID, s.product())
	if err != nil {
		return r, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO client_release_audits(release_id,actor,action) VALUES($1,$2,$3)`, r.ID, uid, action)
	return r, err
}

func (s *Service) Create(ctx context.Context, uid int64, version, notes string, required []string) (Release, error) {
	return s.CreateWithOptions(ctx, uid, version, notes, required, ReleaseOptions{})
}
func (s *Service) CreateWithOptions(ctx context.Context, uid int64, version, notes string, required []string, options ReleaseOptions) (Release, error) {
	if s.product() != ManjuProduct && (options.Mandatory || options.RuntimeHash != "") {
		return Release{}, errors.New("当前产品不支持此更新选项")
	}
	if !VersionValid(version) || len(notes) > 16000 || len(required) == 0 || len(required) > 3 {
		return Release{}, errors.New("版本或目标平台无效")
	}
	seen := map[string]bool{}
	for _, t := range required {
		if seen[t] || !(t == "windows/x64" || t == "macos/universal" || t == "macos/arm64" || t == "macos/x64" || (s.product() == ManjuProduct && t == "renderer/any")) {
			return Release{}, errors.New("目标平台无效")
		}
		seen[t] = true
	}
	b, _ := json.Marshal(required)
	o, _ := json.Marshal(options)
	return scan(s.DB.QueryRowContext(ctx, `INSERT INTO client_releases(product_id,version,notes,required_targets,created_by,release_options) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+columns, s.product(), version, notes, b, uid, o))
}
func (s *Service) Upload(ctx context.Context, id, uid int64, platform, arch, name string, reader io.Reader) (Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Release{}, err
	}
	defer tx.Rollback()
	r, err := s.upload(ctx, tx, id, uid, platform, arch, name, reader)
	if err != nil {
		return r, err
	}
	return r, tx.Commit()
}

func (s *Service) upload(ctx context.Context, tx *sql.Tx, id, uid int64, platform, arch, name string, reader io.Reader) (Release, error) {
	r, e := scan(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM client_releases WHERE id=$1 AND product_id=$2`, id, s.product()))
	if e != nil {
		return r, e
	}
	if !s.canEdit(r) {
		return r, errors.New("当前版本无法上传文件")
	}
	a := Asset{Platform: platform, Arch: arch, Name: name, Size: 1, SHA256: strings.Repeat("0", 64)}
	a.URL = ProductAssetURL(s.product(), r.Version, a.SHA256, name)
	preview := DraftManifest(r)
	preview.Assets = []Asset{a}
	if platform == "renderer" {
		preview.RuntimeHash = strings.Repeat("0", 64)
	}
	if e = preview.Validate(); e != nil {
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
	defer os.Remove(tmp)
	h := sha256.New()
	limit := int64(2 << 30)
	if platform == "renderer" {
		limit = 64 << 20
	}
	n, e := io.Copy(io.MultiWriter(f, h), io.LimitReader(reader, limit+1))
	closeErr := f.Close()
	if e != nil {
		return r, e
	}
	if closeErr != nil {
		return r, closeErr
	}
	if n <= 0 || n > limit {
		return r, errors.New("更新文件为空或超过大小限制")
	}
	a.Size = n
	a.SHA256 = hex.EncodeToString(h.Sum(nil))
	a.URL = ProductAssetURL(s.product(), r.Version, a.SHA256, a.Name)
	runtimeHash := ""
	if platform == "renderer" {
		runtimeHash, e = inspectRendererBundle(tmp, r.Version)
		if e != nil {
			return r, e
		}
	}
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
	status := r.Status
	r, e = s.lockRelease(ctx, tx, id)
	if e != nil {
		return r, e
	}
	if r.Status != status {
		return r, errors.New("版本状态已改变，请重新上传")
	}
	if runtimeHash != "" {
		if r.Status == "published" && runtimeHash != r.Options.RuntimeHash {
			return r, errors.New("替换页面包必须使用相同的桌面运行环境；桌面环境有变化时请发布新版本")
		}
		r.Options.RuntimeHash = runtimeHash
	}
	for i, existing := range r.Assets {
		if existing.Platform == platform && existing.Arch == arch {
			r.Assets[i] = a
			return s.saveChanges(ctx, tx, r, uid, "upload")
		}
	}
	r.Assets = append(r.Assets, a)
	return s.saveChanges(ctx, tx, r, uid, "upload")
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
	product := r.Product
	if product == "" {
		product = Product
	}
	m := Manifest{Schema: 1, Product: product, Version: r.Version, Channel: r.Channel, Sequence: r.ID, PublishedAt: r.CreatedAt.UTC().Format(time.RFC3339), Notes: r.Notes, Assets: r.Assets}
	if product == ManjuProduct {
		m.Schema = 2
		m.AppID = ManjuAppID
		m.Mandatory = r.Options.Mandatory
		m.RuntimeHash = r.Options.RuntimeHash
	}
	return m
}
func matches(r Release, m Manifest) error {
	draft := DraftManifest(r)
	if m.Product != draft.Product || m.Mandatory != draft.Mandatory || m.RuntimeHash != draft.RuntimeHash || m.AppID != draft.AppID || m.Version != r.Version || m.Sequence != r.ID || m.Notes != r.Notes || m.Channel != r.Channel || len(m.Assets) != len(r.Assets) {
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
	m, e := VerifyProduct(env, s.Public, s.product())
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
	m, e := VerifyProduct(*r.Envelope, s.Public, s.product())
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
	if s.product() == ManjuProduct && m.RuntimeHash != "" {
		hasInstaller := false
		for _, asset := range m.Assets {
			if asset.Platform != "renderer" {
				hasInstaller = true
			}
		}
		if !hasInstaller {
			native, err := scan(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM client_releases WHERE product_id=$1 AND status='published' AND EXISTS(SELECT 1 FROM jsonb_array_elements(assets) a WHERE a->>'platform' IN ('macos','windows')) ORDER BY id DESC LIMIT 1`, s.product()))
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if err == nil && native.Options.RuntimeHash != m.RuntimeHash {
				return errors.New("桌面兼容信息已变化，请在此版本中同时提供完整安装包与页面包")
			}
		}
	}
	var previous string
	var previousID int64
	e = tx.QueryRowContext(ctx, `SELECT version,id FROM client_releases WHERE product_id=$1 AND channel='stable' AND published_at IS NOT NULL ORDER BY id DESC LIMIT 1`, s.product()).Scan(&previous, &previousID)
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
	if _, e = tx.ExecContext(ctx, `INSERT INTO client_release_channels(product_id,channel,release_id) VALUES($1,'stable',$2) ON CONFLICT(product_id,channel) DO UPDATE SET release_id=EXCLUDED.release_id`, s.product(), id); e != nil {
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
	result, e := tx.ExecContext(ctx, `UPDATE client_releases SET status='withdrawn' WHERE id=$1 AND product_id=$2 AND status IN ('published','ready')`, id, s.product())
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
	rows, e := s.DB.QueryContext(ctx, `SELECT `+columns+` FROM client_releases WHERE product_id=$1 AND version=$2 AND status='published'`, s.product(), version)
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

// Only this independent product uses server-side signing; Codex++ remains offline-signed.
func (s *Service) Prepare(ctx context.Context, id, uid int64) (Release, error) {
	r, err := s.Get(ctx, id)
	if err != nil {
		return r, err
	}
	signed, err := s.signRelease(r)
	if err != nil {
		return r, err
	}
	return s.Validate(ctx, id, uid, signed)
}

func (s *Service) signRelease(r Release) (Envelope, error) {
	if s.product() != ManjuProduct || len(s.SigningKey) != ed25519.PrivateKeySize {
		return Envelope{}, errors.New("该产品未配置后台发布签名")
	}
	data, err := json.Marshal(DraftManifest(r))
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Manifest: base64.StdEncoding.EncodeToString(data), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(s.SigningKey, data)), KeyID: ManjuKeyID}, nil
}
