package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCountPEMCerts(t *testing.T) {
	dir := t.TempDir()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pemFile := filepath.Join(dir, "ca.pem")
	os.WriteFile(pemFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	derFile := filepath.Join(dir, "ca.cer")
	os.WriteFile(derFile, der, 0o600)

	if n, err := countPEMCerts(pemFile); err != nil || n != 1 {
		t.Fatalf("pem: %d %v", n, err)
	}
	if n, _ := countPEMCerts(derFile); n != 0 {
		t.Fatalf("der should count 0, got %d", n)
	}
	if _, err := countPEMCerts(filepath.Join(dir, "missing.pem")); err == nil {
		t.Fatal("missing file should error")
	}
	if _, err := countPEMCerts(dir); err == nil {
		t.Fatal("directory should error")
	}
}
