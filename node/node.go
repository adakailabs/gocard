package node

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/docker/docker/pkg/stdcopy"

	"github.com/sirupsen/logrus"

	"github.com/adakailabs/gocard/config"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func Start() {

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
	io.Copy(os.Stdout, reader)

	resp, err := cli.ContainerCreate(ctx,
		config.GlobalConfig.ContainerConfig,
		config.GlobalConfig.HostConfig,
		nil, nil, "")
	if err != nil {
		panic(err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}

	WriteContainerID(resp.ID)

	closure := func() {
		timer := time.NewTicker(time.Second)

		for _ = range timer.C {
			logrus.Info("logs...")
			out, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true})
			if err != nil {
				panic(err)
			}
			stdcopy.StdCopy(os.Stdout, os.Stderr, out)
		}
	}

	go closure()

	/*
		statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
		select {
		case err := <-errCh:
			if err != nil {
				panic(err)
			}
		case this := <-statusCh:
			logrus.Info("status: ", this)
		}

		out, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true})
		if err != nil {
			panic(err)
		}

		logrus.Info("out: ", out)

		stdcopy.StdCopy(os.Stdout, os.Stderr, out)

	*/

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

func WriteContainerID(containerID string) {
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
