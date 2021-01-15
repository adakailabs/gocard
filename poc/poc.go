package poc

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/docker/docker/api/types/container"

	"github.com/docker/docker/pkg/stdcopy"

	"github.com/sirupsen/logrus"

	"github.com/adakailabs/gocard/config"

	"github.com/spf13/viper"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func Start() {

	dockerImage := viper.GetString("docker_image")

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

	logrus.Info("container ID: ", resp.ID)

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

}
