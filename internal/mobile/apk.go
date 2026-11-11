package mobile

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Directory, WebRoot, PublicURL, Package, Certificate, AAPT, APKSigner string
	MaxBytes                                                             int64
}

func Load() Config {
	max, _ := strconv.ParseInt(os.Getenv("MOBILE_APK_MAX_BYTES"), 10, 64)
	if max <= 0 || max > 512*1024*1024 {
		max = 150 * 1024 * 1024
	}
	return Config{os.Getenv("MOBILE_RELEASE_DIR"), os.Getenv("MOBILE_WEB_ROOT"), strings.TrimRight(os.Getenv("MOBILE_PUBLIC_URL"), "/"), "id.ponpes.ibnuhafidz.absensi", strings.ToLower(strings.ReplaceAll(os.Getenv("MOBILE_SIGNER_SHA256"), ":", "")), env("MOBILE_AAPT", "aapt"), env("MOBILE_APKSIGNER", "apksigner"), max}
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func Within(parent, child string) bool {
	r, e := filepath.Rel(parent, child)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(os.PathSeparator))
}
func (c Config) Ready() error {
	if !filepath.IsAbs(c.Directory) || !filepath.IsAbs(c.WebRoot) || Within(c.WebRoot, c.Directory) {
		return errors.New("konfigurasi penyimpanan privat di luar web root diperlukan")
	}
	cwd, _ := os.Getwd()
	if Within(cwd, c.Directory) || filepath.Clean(c.Directory) == filepath.VolumeName(c.Directory)+string(os.PathSeparator) {
		return errors.New("direktori APK harus di luar repository dan web root")
	}
	// Resolve existing parents too: symlinked storage must not point back inside a public tree.
	if resolved, e := filepath.EvalSymlinks(c.Directory); e == nil {
		root, _ := filepath.EvalSymlinks(c.WebRoot)
		if root != "" && Within(root, resolved) {
			return errors.New("penyimpanan APK mengarah ke web root")
		}
	}
	u, e := url.Parse(c.PublicURL)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return errors.New("MOBILE_PUBLIC_URL harus origin HTTPS")
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(c.Certificate) {
		return errors.New("sertifikat penandatangan APK belum dikonfigurasi")
	}
	if _, e = exec.LookPath(c.AAPT); e != nil {
		return errors.New("aapt belum tersedia")
	}
	if strings.HasSuffix(strings.ToLower(c.APKSigner), ".jar") {
		if _, e = os.Stat(c.APKSigner); e != nil {
			return errors.New("apksigner.jar belum tersedia")
		}
		_, e = exec.LookPath("java")
	} else {
		_, e = exec.LookPath(c.APKSigner)
	}
	if e != nil {
		return errors.New("apksigner/Java belum tersedia")
	}
	return nil
}

type APKInfo struct {
	Package, Version, Certificate, SHA256 string
	Code, Size                            int64
}

func Digest(path string) (string, int64, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, e
}

type limitedOutput struct{ bytes.Buffer }

func (b *limitedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 256*1024 {
		return 0, io.ErrShortBuffer
	}
	return b.Buffer.Write(p)
}
func run(tool string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if strings.HasSuffix(strings.ToLower(tool), ".jar") {
		args = append([]string{"-jar", tool}, args...)
		tool = "java"
	}
	cmd := exec.CommandContext(ctx, tool, args...)
	out := &limitedOutput{}
	cmd.Stdout = out
	cmd.Stderr = out
	e := cmd.Run()
	if e != nil {
		return "", errors.New("alat pemeriksaan APK gagal; periksa aapt, apksigner dan Java")
	}
	return out.String(), nil
}
func (c Config) Inspect(path string) (APKInfo, error) {
	var info APKInfo
	if e := c.Ready(); e != nil {
		return info, e
	}
	z, e := zip.OpenReader(path)
	if e != nil {
		return info, errors.New("berkas bukan APK ZIP yang valid")
	}
	defer z.Close()
	if len(z.File) > 100000 {
		return info, errors.New("terlalu banyak entri APK")
	}
	manifest := false
	var expanded uint64
	for _, f := range z.File {
		expanded += f.UncompressedSize64
		if expanded > 2*1024*1024*1024 {
			return info, errors.New("APK melebihi batas dekompresi")
		}
		if f.Name == "AndroidManifest.xml" {
			manifest = f.UncompressedSize64 <= 4*1024*1024
		}
	}
	if !manifest {
		return info, errors.New("AndroidManifest.xml tidak valid")
	}
	badging, e := run(c.AAPT, "dump", "badging", path)
	if e != nil {
		return info, e
	}
	fields := regexp.MustCompile(`package: name='([^']+)' versionCode='([0-9]+)' versionName='([^']+)'`).FindStringSubmatch(badging)
	if len(fields) != 4 || strings.Contains(badging, "application-debuggable") {
		return info, errors.New("unggah APK release non-debuggable dengan metadata valid")
	}
	info.Package = fields[1]
	info.Code, _ = strconv.ParseInt(fields[2], 10, 64)
	info.Version = fields[3]
	if info.Package != c.Package || info.Code <= 0 || info.Code > 2100000000 || len(info.Version) > 80 {
		return info, errors.New("package/version APK tidak sesuai aplikasi")
	}
	signed, e := run(c.APKSigner, "verify", "--verbose", "--print-certs", path)
	if e != nil {
		return info, e
	}
	certs := regexp.MustCompile(`Signer #[0-9]+ certificate SHA-256 digest: ([0-9a-fA-F]{64})`).FindAllStringSubmatch(signed, -1)
	if len(certs) != 1 || strings.ToLower(certs[0][1]) != c.Certificate {
		return info, errors.New("tanda tangan APK tidak cocok dengan sertifikat aplikasi")
	}
	info.Certificate = c.Certificate
	info.SHA256, info.Size, e = Digest(path)
	if e != nil {
		return info, e
	}
	if info.Size <= 0 || info.Size > c.MaxBytes {
		return info, fmt.Errorf("ukuran APK tidak diizinkan")
	}
	return info, nil
}
