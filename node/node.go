package node

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/docker/docker/api/types/container"

	"github.com/juju/errors"

	"github.com/sirupsen/logrus"

	"github.com/adakailabs/gocard/config"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func Start() {
	if err := config.GlobalConfig.CheckCardanoConfigFiles(); err != nil {
		logrus.Fatal(errors.ErrorStack(err))
	}

	if config.GlobalConfig.ContainerIsUP {
		logrus.Warn("container is already running")
		return
	}

	dockerImage := config.GlobalConfig.DockerImage

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	reader, err := cli.ImagePull(ctx, dockerImage, types.ImagePullOptions{})
	if err != nil {
		panic(err)
	}
	if _, err = io.Copy(os.Stdout, reader); err != nil {
		err = errors.Annotate(err, "copying to stadout")
		panic(err.Error())
	}

	resp, err := cli.ContainerCreate(ctx,
		config.GlobalConfig.ContainerConfig,
		config.GlobalConfig.HostConfig,
		nil, nil, "")
	if err != nil {
		panic(err)
	}

	if err = cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}

	writeContainerID(resp.ID)

	// readLogs(ctx, cli, resp.ID)

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)

	select {
	case err = <-errCh:
		if err != nil {
			panic(err)
		}
	case this := <-statusCh:
		logrus.Info("status: ", this)
	}
}

func readLogs(ctx context.Context, cli *client.Client, containerID string) {
	out, errC := cli.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true})
	if errC != nil {
		panic(errC)
	}
	scanner := bufio.NewScanner(out)
	done := make(chan struct{})

	timer := time.NewTicker(time.Second * 2)

	closure := func() {
		for range timer.C {
			logrus.Info("monitoring logs")
			out, errC = cli.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true})
			if errC != nil {
				panic(errC)
			}
			for scanner.Scan() {
				logrus.Info("XXXX", scanner.Text())
			}
			done <- struct{}{}
		}
	}
	go closure()

	<-done
}

func Stop() {
	if config.GlobalConfig.ContainerIsUP {
		logrus.Info("attempting to stop container with ID: ", config.GlobalConfig.ContainerID)
		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			panic(err)
		}

		if err := cli.ContainerStop(ctx, config.GlobalConfig.ContainerID, nil); err != nil {
			panic(err)
		}
		logrus.Info("stoped container")
	}
	if config.GlobalConfig.ContainerID != "" {
		err := os.Remove(config.GocardPidFile)
		if err != nil {
			logrus.Error("could not remove pid file")
		}
	} else {
		logrus.Warn("no cardano container is running")
	}
}

func Init() {
	config.GlobalConfig.SetCardanoInit()
}

func writeContainerID(containerID string) {
	logrus.Info("container ID: ", containerID)
	f, err := os.Create(config.GocardPidFile)
	if err != nil {
		logrus.Error("could not create container ID file: ", err.Error())
		panic(err.Error())
	}
	_, err = f.Write([]byte(fmt.Sprintf("container_id: %s", containerID)))
	if err != nil {
		logrus.Error("could not write to container ID file: ", err.Error())
		panic(err.Error())
	}
	err = f.Close()
	if err != nil {
		logrus.Error("could not close container ID file")
	}
}
