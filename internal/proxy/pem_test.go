package proxy

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

func encodePEM(blockType string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
}

func encodeKey(key any) ([]byte, error) {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return encodePEM("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(k)), nil
	case *ecdsa.PrivateKey:
		der, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		return encodePEM("EC PRIVATE KEY", der), nil
	default:
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, errors.New("unsupported private key type")
		}
		return encodePEM("PRIVATE KEY", der), nil
	}
}
