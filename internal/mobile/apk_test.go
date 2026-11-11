package mobile

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDigestAndPrivateStorage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "test.apk")
	require.NoError(t, os.WriteFile(file, []byte("abc"), 0600))
	hash, size, e := Digest(file)
	require.NoError(t, e)
	require.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", hash)
	require.EqualValues(t, 3, size)
	cfg := Config{Directory: t.TempDir(), WebRoot: t.TempDir(), PublicURL: "https://ibnuhafidz.ponpes.id", Certificate: strings.Repeat("a", 64), AAPT: "go", APKSigner: "go"}
	require.NoError(t, cfg.Ready())
	cfg.Directory = filepath.Join(cfg.WebRoot, "uploads")
	require.Error(t, cfg.Ready())
}
func TestInvalidAPKRejected(t *testing.T) {
	cfg := Config{Directory: t.TempDir(), WebRoot: t.TempDir(), PublicURL: "https://ibnuhafidz.ponpes.id", Certificate: strings.Repeat("a", 64), AAPT: "go", APKSigner: "go"}
	file := filepath.Join(cfg.Directory, "fake.apk")
	require.NoError(t, os.WriteFile(file, []byte("not an apk"), 0600))
	_, e := cfg.Inspect(file)
	require.ErrorContains(t, e, "ZIP")
}
func TestRealSignedAPK(t *testing.T) {
	path := os.Getenv("IBNU_TEST_APK")
	if path == "" {
		t.Skip("optional local signed APK/toolchain fixture not supplied")
	}
	cfg := Load()
	info, e := cfg.Inspect(path)
	require.NoError(t, e)
	require.Equal(t, cfg.Package, info.Package)
	require.Equal(t, cfg.Certificate, info.Certificate)
	require.Greater(t, info.Code, int64(0))
	require.Len(t, info.SHA256, 64)
}
