package clientupdates

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func manjuArchive(t *testing.T, version string) []byte {
	t.Helper()
	contents := []byte("<!doctype html><h1>笑笑漫剧更新验证</h1>")
	digest := sha256.Sum256(contents)
	metadata, err := json.Marshal(map[string]any{"schema_version": 1, "app_id": ManjuAppID, "version": version, "runtime_sha256": strings.Repeat("a", 64), "files": map[string]any{"index.html": map[string]any{"size_bytes": len(contents), "sha256": hex.EncodeToString(digest[:])}}})
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for name, data := range map[string][]byte{"index.html": contents, "renderer-update.json": metadata} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestManjuReleaseLifecycleAndProductIsolation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to an isolated local PostgreSQL database")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatal("release integration tests require local PostgreSQL")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := "updates_test_" + hex.EncodeToString(random[:])
	if _, err = admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)
	ctx := context.Background()
	if _, err = db.Exec(`CREATE TABLE users(id bigint PRIMARY KEY,role text,status text,deleted_at timestamptz); INSERT INTO users VALUES(7,'admin','active',NULL),(8,'user','active',NULL),(9,'admin','disabled',NULL)`); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	codex, err := New(ctx, db, db, t.TempDir(), public)
	if err != nil {
		t.Fatal(err)
	}
	manju, err := New(ctx, db, db, t.TempDir(), public)
	if err != nil {
		t.Fatal(err)
	}
	manju.Product = ManjuProduct
	manju.SigningKey = private
	if !manju.IsAdmin(ctx, 7) || manju.IsAdmin(ctx, 8) || manju.IsAdmin(ctx, 9) {
		t.Fatal("administrator role or status checks changed")
	}
	old, err := codex.Create(ctx, 7, "0.2.13", "Codex existing release", []string{"windows/x64"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := manju.CreateWithOptions(ctx, 7, "0.2.13", "页面优化", []string{"renderer/any"}, ReleaseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manju.Get(ctx, old.ID); err != sql.ErrNoRows {
		t.Fatal("cross-product release ID was exposed")
	}
	if _, err = codex.Get(ctx, r.ID); err != sql.ErrNoRows {
		t.Fatal("Codex exposed a Manju release")
	}
	if _, err = manju.Prepare(ctx, r.ID, 7); err == nil {
		t.Fatal("release with missing required asset was signed")
	}
	archive := manjuArchive(t, r.Version)
	r, err = manju.Upload(ctx, r.ID, 7, "renderer", "any", "Manju-renderer.zip", bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	r, err = manju.EditOptions(ctx, r.ID, 7, "正式更新记录", true)
	if err != nil {
		t.Fatal(err)
	}
	r, err = manju.Prepare(ctx, r.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	m, err := VerifyProduct(*r.Envelope, public, ManjuProduct)
	if err != nil || !m.Mandatory || m.Notes != "正式更新记录" || m.RuntimeHash != strings.Repeat("a", 64) {
		t.Fatalf("signed release policy did not match draft: %v", err)
	}
	if _, err = Verify(*r.Envelope, public); err == nil {
		t.Fatal("Manju envelope was accepted as a Codex release")
	}
	if _, err = manju.Download(ctx, r.Version, r.Assets[0].SHA256, r.Assets[0].Name); err == nil {
		t.Fatal("unpublished asset was publicly available")
	}
	if err = manju.Publish(ctx, r.ID, 7); err != nil {
		t.Fatal(err)
	}
	current, err := manju.Current(ctx)
	if err != nil || current.ID != r.ID {
		t.Fatal("published channel did not advance", err)
	}
	if _, err = codex.Current(ctx); err != sql.ErrNoRows {
		t.Fatal("Manju publication changed the Codex channel")
	}
	if _, err = manju.EditOptions(ctx, r.ID, 7, "rewrite", false); err == nil {
		t.Fatal("published signed policy could be changed")
	}
	file, err := manju.Download(ctx, r.Version, r.Assets[0].SHA256, r.Assets[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(saved, archive) {
		t.Fatal("published bytes changed", err)
	}
	if err = manju.Withdraw(ctx, old.ID, 7, "wrong product"); err == nil {
		t.Fatal("cross-product withdrawal succeeded")
	}
	if err = manju.Withdraw(ctx, r.ID, 7, "验证撤回"); err != nil {
		t.Fatal(err)
	}
	if _, err = manju.Current(ctx); err != sql.ErrNoRows {
		t.Fatal("withdrawn release remained current")
	}
	if _, err = manju.Download(ctx, r.Version, r.Assets[0].SHA256, r.Assets[0].Name); err == nil {
		t.Fatal("withdrawn asset still downloadable")
	}

	// The pre-existing offline-signed Codex flow still works with the additive schema.
	old, err = codex.Upload(ctx, old.ID, 7, "windows", "x64", "Codex-setup.exe", bytes.NewReader([]byte("legacy installer fixture")))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(DraftManifest(old))
	legacy := Envelope{Manifest: base64.StdEncoding.EncodeToString(body), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, body)), KeyID: "fastaiapi-release-v1"}
	if _, err = codex.Validate(ctx, old.ID, 7, legacy); err != nil {
		t.Fatal(err)
	}
	if err = codex.Publish(ctx, old.ID, 7); err != nil {
		t.Fatal(err)
	}
	if _, err = codex.Prepare(ctx, old.ID, 7); err == nil {
		t.Fatal("Codex private-key policy was changed")
	}

	// Upload a native installer through multiple CDN-safe chunks; retries do not duplicate bytes.
	native, err := manju.Create(ctx, 7, "0.2.14", "完整更新", []string{"windows/x64"})
	if err != nil {
		t.Fatal(err)
	}
	content := bytes.Repeat([]byte("x"), int(UploadChunkBytes)+29)
	copy(content, []byte("MZ"))
	id := fmt.Sprintf("%032x", 2)
	progress, err := manju.UploadChunk(ctx, native.ID, 7, id, "windows", "x64", "Manju-setup.exe", 0, int64(len(content)), bytes.NewReader(content[:UploadChunkBytes]))
	if err != nil || progress.Complete || progress.NextOffset != UploadChunkBytes {
		t.Fatal("first chunk failed", err)
	}
	if _, err = manju.UploadChunk(ctx, native.ID, 8, id, "windows", "x64", "Manju-setup.exe", UploadChunkBytes, int64(len(content)), bytes.NewReader(content[UploadChunkBytes:])); err == nil {
		t.Fatal("another administrator identity hijacked an upload session")
	}
	if _, err = manju.UploadChunk(ctx, native.ID, 7, id, "windows", "x64", "Manju-setup.exe", 0, int64(len(content)), bytes.NewReader(content[:UploadChunkBytes])); err != nil {
		t.Fatal("retry was not idempotent", err)
	}
	progress, err = manju.UploadChunk(ctx, native.ID, 7, id, "windows", "x64", "Manju-setup.exe", UploadChunkBytes, int64(len(content)), bytes.NewReader(content[UploadChunkBytes:]))
	if err != nil || !progress.Complete || progress.Release == nil {
		t.Fatal("final chunk failed", err)
	}
	r = *progress.Release
	hash := sha256.Sum256(content)
	if r.Assets[0].SHA256 != hex.EncodeToString(hash[:]) || r.Assets[0].Size != int64(len(content)) {
		t.Fatal("chunked upload changed the original installer")
	}
	if _, err = manju.Prepare(ctx, r.ID, 7); err != nil {
		t.Fatal(err)
	}
	if err = manju.Publish(ctx, r.ID, 7); err != nil {
		t.Fatal(err)
	}
	installer, err := manju.InstallerRelease(ctx)
	if err != nil || installer.ID != r.ID {
		t.Fatal("download page did not select the latest complete installer", err)
	}
	page, err := manju.Create(ctx, 7, "0.2.15", "页面更新", []string{"renderer/any"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manju.Upload(ctx, page.ID, 7, "renderer", "any", "page.zip", bytes.NewReader(manjuArchive(t, page.Version))); err != nil {
		t.Fatal(err)
	}
	if _, err = manju.Prepare(ctx, page.ID, 7); err != nil {
		t.Fatal(err)
	}
	if err = manju.Publish(ctx, page.ID, 7); err == nil {
		t.Fatal("incompatible page-only release was published without a matching installer")
	}
}

func TestRendererArchiveRejectsWrongIdentityAndTraversal(t *testing.T) {
	for _, name := range []string{"../index.html", "assets/../../escape", "assets/CON.js", "/index.html", "assets/a\\b.js"} {
		if rendererNameValid(name) {
			t.Fatal("unsafe path accepted", name)
		}
	}
	file, err := os.CreateTemp(t.TempDir(), "bundle-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(file, bytes.NewReader(manjuArchive(t, "0.2.13"))); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err = inspectRendererBundle(file.Name(), "0.2.14"); err == nil {
		t.Fatal("wrong renderer version accepted")
	}
}
