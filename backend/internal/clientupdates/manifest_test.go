package clientupdates

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture() Manifest {
	hash := strings.Repeat("a", 64)
	return Manifest{Schema: 1, Product: Product, Version: "1.11.3", Channel: "stable", Sequence: 3, Assets: []Asset{{Platform: "macos", Arch: "universal", Name: "CodexPlusPlus-1.11.3-macos-universal.dmg", Size: 100, SHA256: hash, URL: AssetURL("1.11.3", hash, "CodexPlusPlus-1.11.3-macos-universal.dmg")}}}
}
func TestVerifyRejectsTamperAndExternalDownloads(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	m := fixture()
	data, _ := json.Marshal(m)
	env := Envelope{Manifest: base64.StdEncoding.EncodeToString(data), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, data)), KeyID: "fastaiapi-release-v1"}
	if _, e := Verify(env, public); e != nil {
		t.Fatal(e)
	}
	env.Manifest = base64.StdEncoding.EncodeToString(append(data, ' '))
	if _, e := Verify(env, public); e == nil {
		t.Fatal("tampered accepted")
	}
	m.Assets[0].URL = "https://github.com/owner/release.dmg"
	if e := m.Validate(); e == nil {
		t.Fatal("external URL accepted")
	}
	m = fixture()
	m.Assets[0].Name = "../escape.dmg"
	if e := m.Validate(); e == nil {
		t.Fatal("traversal accepted")
	}
}
func TestRequiredTargetsAndDraftMatch(t *testing.T) {
	m := fixture()
	r := Release{ID: 3, Version: m.Version, Channel: "stable", Assets: m.Assets, Required: []string{"macos/universal", "windows/x64"}}
	if matches(r, m) == nil {
		t.Fatal("missing Windows accepted")
	}
	r.Required = []string{"macos/universal"}
	if e := matches(r, m); e != nil {
		t.Fatal(e)
	}
	m.Sequence = 4
	if matches(r, m) == nil {
		t.Fatal("wrong draft accepted")
	}
}
func TestVerifyFileRejectsCorruption(t *testing.T) {
	s := Service{Root: t.TempDir()}
	a := fixture().Assets[0]
	dir := filepath.Join(s.Root, "objects", a.SHA256)
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, a.Name), []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if s.verifyFile(a) == nil {
		t.Fatal("bad digest accepted")
	}
}
func TestVersionOrdering(t *testing.T) {
	if !Newer("1.11.3", "1.11.2") || Newer("1.11.2", "1.11.3") || Newer("1.11.3-beta", "1.11.2") || Newer("1.11.2", "1.11.2") {
		t.Fatal("version ordering incorrect")
	}
}
