package config

import "C"
import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/tidwall/sjson"

	"github.com/juju/errors"

	"github.com/tidwall/gjson"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var GlobalConfig *Config

const GocardPidFile = "/tmp/gocard.pid.yam"

type Config struct {
	NodeName        string
	NodeTicker      string
	ContainerName   string
	IsProducer      bool
	DockerImage     string
	Mounts          []mount.Mount
	PortSet         nat.PortSet
	HostConfig      *container.HostConfig
	ContainerConfig *container.Config

	CardanoBaseContainer string
	CardanoBaseLocal     string
	CardanoDb            string
	CardanoCli           string
	CardanoSocket        string
	CardanoPort          string
	CardanoHostAddress   string
	CardanoCmdStrings    []string

	ContainerID   string
	ContainerIsUP bool
}

func LoadConfig() {

	c := &Config{}

	c.NodeName = viper.GetString("node_name")
	c.NodeTicker = viper.GetString("node_ticker")
	c.DockerImage = viper.GetString("docker_image")
	c.IsProducer = viper.GetBool("service_is_producer")
	c.ContainerName = viper.GetString("server_name")
	c.SetCardanoPaths()
	c.SetExposedPorts()
	c.SetMount()
	c.SetContainerName()
	c.SetCmdStrings()
	c.SetHostConfig()
	c.SetContainerConfig()
	//c.LoadCardanoViperConfig()
	GlobalConfig = c
}

func (c *Config) LogConfig() {
	containerType := "relay"
	if c.IsProducer {
		containerType = "producer"
	}
	logrus.Info("container type: ", containerType)
	logrus.Info("docker image: ", GlobalConfig.DockerImage)

	logrus.Info("cardano base container: ", c.CardanoBaseContainer)
	logrus.Info("cardano base local    : ", c.CardanoBaseLocal)
	logrus.Info("cardano db: ", c.CardanoDb)
	logrus.Info("cardano socket: ", c.CardanoSocket)
	logrus.Info("cardano host: ", c.CardanoHostAddress)
	logrus.Info("cardano port: ", c.CardanoPort)
	logrus.Info("cardano cmd: ", c.CardanoCmdStrings)
	for key, _ := range c.PortSet {
		logrus.Info("exposed port: ", key)
	}

}

func (c *Config) LoadCardanoViperConfig() {
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
		if !skipDefaults {
			element := `[
    %s,
    [
      "FileSK",
      "/tmp/cardano-node/log/cardano.log"
    ]
  ]`
			newJSONElement := fmt.Sprintf(element, defaultScribes[0].Raw)
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
      "scName": "/tmp/cardano-node/log/cardano.log",
      "scRotation": null
    }
  ]`
			newJSONElement := fmt.Sprintf(element, setupScribes[0].Raw)
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

		ioutil.WriteFile(cardanoConfigFile, []byte(newJSON), os.ModePerm)
	}
}

func (c *Config) SetCardanoPaths() {
	c.CardanoBaseContainer = viper.GetString("cardano_base_container")
	c.CardanoBaseLocal = viper.GetString("cardano_base_local")
	c.CardanoCli = viper.GetString("cardano_cli")
	c.CardanoDb = viper.GetString("cardano_db")
	c.CardanoSocket = viper.GetString("cardano_socket")
	c.CardanoHostAddress = viper.GetString("cardano_host_address")
	c.CardanoPort = viper.GetString("cardano_port")

	if _, err := os.Stat(c.CardanoBaseLocal); os.IsNotExist(err) {
		os.MkdirAll(c.CardanoBaseLocal, os.ModePerm)
	}
	c.CheckDockerContainerUp()
}

func (c *Config) SetContainerName() {
	sufix := "Relay"
	if c.IsProducer {
		sufix = "Producer"
	}
	c.ContainerName = fmt.Sprintf("%s%s", c.ContainerName, sufix)
}

func (c *Config) SetMount() {
	c.Mounts = []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: c.CardanoBaseLocal,
			Target: c.CardanoBaseContainer,
		},
	}
}

func (c *Config) SetExposedPorts() {
	c.PortSet = make(map[nat.Port]struct{})
	ports := viper.GetStringSlice("expose_ports")
	if !c.IsProducer {
		cardanoPort := fmt.Sprintf("%s/tcp", c.CardanoPort)
		ports = append(ports, cardanoPort)
	}
	for _, port := range ports {
		c.PortSet[nat.Port(port)] = struct{}{}
	}
}

func (c *Config) SetHostConfig() {
	c.HostConfig = &container.HostConfig{
		Mounts: c.Mounts,
	}
}

func (c *Config) SetCmdStrings() {

	c.CardanoCmdStrings = make([]string, 0, 10)

	dataBasePathS := fmt.Sprintf("--database-path")
	dataBasePathC := fmt.Sprintf("%s%s", c.CardanoBaseContainer, c.CardanoDb)
	socketPathS := fmt.Sprintf("--socket-path")
	socketPathC := fmt.Sprintf("%s%s", c.CardanoBaseContainer, c.CardanoSocket)
	portS := fmt.Sprintf("--port")
	portC := fmt.Sprintf("%s", c.CardanoPort)
	hostAddrS := fmt.Sprintf("--host-addr")
	hostAddrC := fmt.Sprintf("%s", c.CardanoHostAddress)
	cConfigS := fmt.Sprintf("--config")
	cConfigC := fmt.Sprintf("%s/config/config.json", c.CardanoBaseContainer)
	topologyS := fmt.Sprintf("--topology")
	topologyC := fmt.Sprintf("%s/config/topology.json", c.CardanoBaseContainer)

	//c.CardanoCmdStrings = append(c.CardanoCmdStrings, "entrypoint.sh")
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, "run")
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, dataBasePathS)
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, dataBasePathC)
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, socketPathS)
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, socketPathC)
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, portS)
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, portC)
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, hostAddrS)
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, hostAddrC)
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, cConfigS)
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, cConfigC)
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, topologyS)
	c.CardanoCmdStrings = append(c.CardanoCmdStrings, topologyC)

}

func (c *Config) SetContainerConfig() {
	c.ContainerConfig = &container.Config{
		Hostname:     c.ContainerName,
		Image:        c.DockerImage,
		Cmd:          c.CardanoCmdStrings,
		Tty:          false,
		ExposedPorts: c.PortSet,
	}
}

func (c *Config) CheckDockerContainerUp() {
	c.ContainerID = viper.GetString("container_id")
	if c.ContainerID != "" {
		logrus.Info("container ID found: ", c.ContainerID)

		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			panic(err)
		}

		containers, err := cli.ContainerList(ctx, types.ContainerListOptions{})
		if err != nil {
			panic(err)
		}

		for _, container := range containers {
			if container.ID == c.ContainerID {
				logrus.Info("container is running")
				c.ContainerIsUP = true
			} else {
				logrus.Info("container is not running")
			}
		}

		if !c.ContainerIsUP {
			os.Remove(GocardPidFile)
		}

	}
}
