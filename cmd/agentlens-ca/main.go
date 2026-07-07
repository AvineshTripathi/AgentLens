package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get home dir: %v", err)
	}

	caDir := filepath.Join(homeDir, ".agentlens")
	if err := os.MkdirAll(caDir, 0700); err != nil {
		log.Fatalf("Failed to create CA directory: %v", err)
	}

	caCertPath := filepath.Join(caDir, "ca.crt")
	caKeyPath := filepath.Join(caDir, "ca.key")

	if _, err := os.Stat(caCertPath); err == nil {
		if _, err := os.Stat(caKeyPath); err == nil {
			fmt.Printf("CA already exists at %s\n", caDir)
			return
		}
	}

	fmt.Println("Generating new AgentLens Root CA...")

	// Generate private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("Failed to generate private key: %v", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		log.Fatalf("Failed to generate serial number: %v", err)
	}

	caTemplate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:       []string{"AgentLens"},
			OrganizationalUnit: []string{"AgentLens MITM Proxy"},
			CommonName:         "AgentLens Root CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 years
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Create the certificate
	caBytes, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &priv.PublicKey, priv)
	if err != nil {
		log.Fatalf("Failed to create certificate: %v", err)
	}

	// Write certificate
	certOut, err := os.Create(caCertPath)
	if err != nil {
		log.Fatalf("Failed to open cert.pem for writing: %v", err)
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: caBytes})
	fmt.Printf("Wrote %s\n", caCertPath)

	// Write private key
	keyOut, err := os.OpenFile(caKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Failed to open key.pem for writing: %v", err)
	}
	defer keyOut.Close()
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	fmt.Printf("Wrote %s\n", caKeyPath)

	fmt.Println("\nSuccess! To use this CA, you must:")
	fmt.Println("1. Trust it in your system keychain (macOS/Windows/Linux)")
	fmt.Println("2. Or set NODE_EXTRA_CA_CERTS if using Node.js agents like claude-code:")
	fmt.Printf("   export NODE_EXTRA_CA_CERTS=\"%s\"\n", caCertPath)
}
