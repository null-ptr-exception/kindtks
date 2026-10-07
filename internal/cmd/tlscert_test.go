package cmd

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestWriteSelfSignedCert(t *testing.T) {
	dir := t.TempDir()
	names := []string{"kindtks.localhost", "*.kindtks.localhost", "kindtks.local", "*.kindtks.local"}
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	if err := writeSelfSignedCert(dir, names, now); err != nil {
		t.Fatal(err)
	}

	pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key"))
	if err != nil {
		t.Fatalf("cert and key do not match: %v", err)
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != "kindtks.localhost" {
		t.Errorf("CN = %q", cert.Subject.CommonName)
	}
	if !slices.Equal(cert.DNSNames, names) {
		t.Errorf("SANs = %v", cert.DNSNames)
	}
	if err := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); err != nil {
		t.Errorf("not self-signed: %v", err)
	}
	if !cert.NotAfter.Equal(now.AddDate(10, 0, 0)) {
		t.Errorf("NotAfter = %v", cert.NotAfter)
	}
	fi, err := os.Stat(filepath.Join(dir, "tls.key"))
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Errorf("key mode = %v, %v", fi.Mode().Perm(), err)
	}
}

func TestTLSCertCommand_Hidden(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"tls-cert"})
	if err != nil || c.Name() != "tls-cert" || !c.Hidden {
		t.Fatalf("tls-cert not registered as hidden: %v", err)
	}
}
