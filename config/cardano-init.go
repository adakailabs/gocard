package config

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/juju/errors"
	"github.com/spf13/viper"
)

const config = "mainnet-config.json"
const newConfig = "config.json"
const shelleyGenesis = "mainnet-shelley-genesis.json"
const newShelleyGenesis = "mainnet-shelley-genesis.json"
const byronGenesis = "mainnet-byron-genesis.json"
const newByronGenesis = "mainnet-byron-genesis.json"
const topology = "mainnet-topology.json"
const newTopology = "topology.json"

var cardanoConfigFiles = map[string]string{
	config:         newConfig,
	shelleyGenesis: newShelleyGenesis,
	byronGenesis:   newByronGenesis,
	topology:       newTopology,
}

func (c *Config) SetCardanoInit() {

	configPath := fmt.Sprintf("%s/%s", c.CardanoBaseLocal, "config")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		os.MkdirAll(configPath, os.ModePerm)
	}

	configURL := viper.GetString("cardano_latest_config")
	if configURL == "" {
		err := fmt.Errorf("cardano latest config URL not specified")
		errors.Annotate(err, "")
		log.Fatal(errors.ErrorStack(err))
	} else {
		for urlName, newName := range cardanoConfigFiles {
			err := downloadFile(newName, configPath, urlName, configURL)
			if err != nil {
				err = errors.Annotatef(err, "while downloading file: %s", newName)
			}
		}

	}

	c.LoadCardanoViperConfig()

}

func (c *Config) CheckCardanoConfigFiles() error {
	configPath := fmt.Sprintf("%s/%s", c.CardanoBaseLocal, "config")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		err = errors.Annotate(err, "cardano configuration directory does not exist")
		return err
	}

	for _, newName := range cardanoConfigFiles {
		filePath := fmt.Sprintf("%s/%s", configPath, newName)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			err = errors.Annotatef(err, "file %s not found in config dir", filePath)
			return err
		}
	}

	return nil
}
func downloadFile(newFileName, dirPath string, urlFileName, fullUrl string) error {
	filePath := fmt.Sprintf("%s/%s", dirPath, newFileName)
	url := fmt.Sprintf("%s%s", fullUrl, urlFileName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logrus.Info("downloading file from: ", url)
		logrus.Info("writing to: ", filePath)
		// Get the data
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// Create the file
		out, err := os.Create(filePath)
		if err != nil {
			return err
		}
		defer out.Close()

		// Write the body to file
		_, err = io.Copy(out, resp.Body)
		return err
	} else {
		logrus.Info("found file: ", filePath)
	}

	return nil

}
