package config

import "C"
import (
	"context"
	"fmt"
	"github.com/juju/errors"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const GocardPidFile = "/tmp/gocard.pid.yaml"

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
	CardanoDB            string
	CardanoCli           string
	CardanoSocket        string
	CardanoPort          string
	CardanoHostAddress   string
	CardanoCmdStrings    []string

	ContainerID   string
	ContainerIsUP bool
}


func New() *Config {
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
	c.LogConfig()
	return c
}

func (c *Config) LogConfig() {
	containerType := "relay"
	if c.IsProducer {
		containerType = "producer"
	}
	logrus.Info("container type: ", containerType)
	logrus.Info("docker image: ", c.DockerImage)

	logrus.Info("cardano base container: ", c.CardanoBaseContainer)
	logrus.Info("cardano base local    : ", c.CardanoBaseLocal)
	logrus.Info("cardano db: ", c.CardanoDB)
	logrus.Info("cardano socket: ", c.CardanoSocket)
	logrus.Info("cardano host: ", c.CardanoHostAddress)
	logrus.Info("cardano port: ", c.CardanoPort)
	logrus.Info("cardano cmd: ", c.CardanoCmdStrings)
	for key := range c.PortSet {
		logrus.Info("exposed port: ", key)
	}
}


func (c *Config) SetCardanoPaths() {
	c.CardanoBaseContainer = viper.GetString("cardano_base_container")
	c.CardanoBaseLocal = viper.GetString("cardano_base_local")
	c.CardanoCli = viper.GetString("cardano_cli")
	c.CardanoDB = viper.GetString("cardano_db")
	c.CardanoSocket = viper.GetString("cardano_socket")
	c.CardanoHostAddress = viper.GetString("cardano_host_address")
	c.CardanoPort = viper.GetString("cardano_port")

	if _, err := os.Stat(c.CardanoBaseLocal); os.IsNotExist(err) {
		if err := os.MkdirAll(c.CardanoBaseLocal, os.ModePerm); err != nil {
			err = errors.Annotatef(err,"creating dir path: %s", c.CardanoBaseLocal)
			panic(err.Error())
		}
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
		PortBindings: nat.PortMap{
			"9100/tcp": []nat.PortBinding{
            {
                HostIP: "0.0.0.0",
                HostPort: "9100",
            },
        },
			"12798/tcp": []nat.PortBinding{
				{
					HostIP: "0.0.0.0",
					HostPort: "9100",
				},
			},
			"3001/tcp": []nat.PortBinding{
				{
					HostIP: "0.0.0.0",
					HostPort: "9100",
				},
			},

    },
	}
}

func (c *Config) SetCmdStrings() {
	c.CardanoCmdStrings = make([]string, 0, 10)

	dataBasePathS := "--database-path"
	dataBasePathC := fmt.Sprintf("%s%s", c.CardanoBaseContainer, c.CardanoDB)
	socketPathS := "--socket-path"
	socketPathC := fmt.Sprintf("%s%s", c.CardanoBaseContainer, c.CardanoSocket)
	portS := "--port"
	portC :=  c.CardanoPort
	hostAddrS := "--host-addr"
	hostAddrC := c.CardanoHostAddress
	cConfigS := "--config"
	cConfigC := fmt.Sprintf("%s/config/config.json", c.CardanoBaseContainer)
	topologyS := "--topology"
	topologyC := fmt.Sprintf("%s/config/topology.json", c.CardanoBaseContainer)

	c.CardanoCmdStrings = append(c.CardanoCmdStrings, "run",
		dataBasePathS,
		dataBasePathC,

		socketPathS,
		socketPathC,

		portS,
		portC,

		hostAddrS,
		hostAddrC,

		cConfigS,
		cConfigC,

		topologyS,
		topologyC)
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

		for i := range containers {
			if containers[i].ID == c.ContainerID {
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
