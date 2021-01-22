package config

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/sirupsen/logrus"

	"github.com/juju/errors"
	"github.com/spf13/viper"
)

const NodeTypeRelay = "relay"
const NodeTypeProducer = "producer"

const cardanoConfigURL = "https://hydra.iohk.io/job/Cardano/iohk-nix/cardano-deployment/latest-finished/download/1/"
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
	rtViewPath := fmt.Sprintf("%s/%s", c.CardanoBaseLocal, "rt-view")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.MkdirAll(configPath, os.ModePerm); err != nil {
			err = errors.Annotatef(err, "creating dir: %s", configPath)
			panic(err.Error())
		}
	}

	if _, err := os.Stat(rtViewPath); os.IsNotExist(err) {
		if err := os.MkdirAll(rtViewPath, os.ModePerm); err != nil {
			err = errors.Annotatef(err, "creating dir: %s", configPath)
			panic(err.Error())
		}
	}

	configURL := viper.GetString("cardano_latest_config")
	if configURL == "" {
		err := fmt.Errorf("cardano latest config URL not specified")
		err = errors.Annotate(err, "")
		log.Fatal(errors.ErrorStack(err))
	} else {
		for urlName, newName := range cardanoConfigFiles {
			err := downloadFile(newName, configPath, urlName)
			if err != nil {
				err = errors.Annotatef(err, "while downloading file: %s", newName)
				panic(err.Error())
			}
		}
	}

	c.updateCardanoConfig()
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

func (c *Config) updateCardanoConfig() {
	cardanoConfigDir := fmt.Sprintf("%s/config", c.CardanoBaseLocal)
	cardanoConfigFile := fmt.Sprintf("%s/config.json", cardanoConfigDir)
	var newJSON string
	if _, err := os.Stat(cardanoConfigFile); err == nil {
		jsonFile, err := ioutil.ReadFile(cardanoConfigFile)
		if err != nil {
			err = errors.Annotate(err, "could not read cardano config file")
			panic(errors.ErrorStack(err))
		}
		defaultScribes := gjson.Get(string(jsonFile), "defaultScribes").Array()
		skipDefaults := false
		for _, defaults := range defaultScribes {
			defaultsWithType := defaults.Array()
			for _, element := range defaultsWithType {
				elementWithType := element.Str
				if elementWithType == "FileSK" {
					skipDefaults = true
				}
			}
		}

		nodeType := NodeTypeRelay
		if c.IsProducer {
			nodeType = NodeTypeProducer
		}

		nodeName := fmt.Sprintf("%s-%s", c.ContainerName, nodeType)

		cardanoLogPath := fmt.Sprintf("%s/log/cardano-%s.log", c.CardanoBaseContainer, nodeName)

		if !skipDefaults {
			element := `[
    %s,
    [
      "FileSK",
      "%s"
    ]
  ]`
			newJSONElement := fmt.Sprintf(element, defaultScribes[0].Raw, cardanoLogPath)
			newJSON, err = sjson.SetRaw(string(jsonFile), "defaultScribes", newJSONElement)
			if err != nil {
				panic(err.Error())
			}
		}

		setupScribes := gjson.Get(newJSON, "setupScribes").Array()
		skipSetup := false
		for _, setupUps := range setupScribes {
			if strings.Contains(setupUps.Raw, "FileSK") {
				skipSetup = true
			}
		}
		time.Sleep(time.Second)
		if !skipSetup {
			element := `[
    %s,
    {
      "scFormat": "ScText",
      "scKind": "FileSK",
      "scName": "%s",
      "scRotation": null
    }
  ]`
			newJSONElement := fmt.Sprintf(element, setupScribes[0].Raw, cardanoLogPath)
			newJSON, err = sjson.SetRaw(newJSON, "setupScribes", newJSONElement)
			if err != nil {
				panic(err.Error())
			}
		}

		prometheusJSON := fmt.Sprintf(`[
    "%s",
    %d
  ]`,
			viper.GetString("cardano_hasprometheus.address"),
			viper.GetInt("cardano_hasprometheus.port"))

		newJSON, err = sjson.SetRaw(newJSON, "hasPrometheus", prometheusJSON)
		if err != nil {
			panic(err.Error())
		}

		if err := ioutil.WriteFile(cardanoConfigFile, []byte(newJSON), os.ModePerm); err != nil {
			panic(errors.Annotatef(err, "writing to file %s", cardanoConfigFile).Error())
		}
	}
}

func downloadFile(newFileName, dirPath, urlFileName string) error {
	filePath := fmt.Sprintf("%s/%s", dirPath, newFileName)
	url := fmt.Sprintf("%s%s", cardanoConfigURL, urlFileName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logrus.Info("downloading file from: ", url)
		logrus.Info("writing to: ", filePath)
		// Get the data
		resp, err := http.Get(fmt.Sprintf("%s%s", cardanoConfigURL, urlFileName))
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
	}

	logrus.Info("found file: ", filePath)

	return nil
}
