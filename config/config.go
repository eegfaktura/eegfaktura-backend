package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"io/ioutil"
)

func ReadConfig(path string) {
	viper.SetConfigName("config")
	viper.AddConfigPath(path)
	viper.AutomaticEnv()
	viper.SetConfigType("yml")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file, %s", err)
	}
}

func ReadCertificate(publicKeyFile string) (*rsa.PublicKey, error) {
	pubData, err := RSAKeyFile(publicKeyFile)
	if err != nil {
		return nil, err
	}
	pub, err := RSAPublicKey(pubData)
	if err != nil {
		return nil, err
	}

	return pub, nil
}

func RSAKeyFile(file string) (data []byte, err error) {

	log.Printf("Key-File: %s", file)
	data, err = ioutil.ReadFile(file)
	return
}

// RSAPublicKey parses data as *rsa.PublicKey
func RSAPublicKey(data []byte) (*rsa.PublicKey, error) {
	input := pemDecode(data)

	var err error
	var key interface{}

	if key, err = x509.ParsePKIXPublicKey(input); err != nil {
		if cert, err := x509.ParseCertificate(input); err == nil {
			key = cert.PublicKey
		} else {
			return nil, err
		}
	}

	return key.(*rsa.PublicKey), nil
}

func pemDecode(data []byte) []byte {
	if block, _ := pem.Decode(data); block != nil {
		return block.Bytes
	}

	return data
}
