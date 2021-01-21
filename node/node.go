package node

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"

	"github.com/coreos/go-systemd/daemon"

	"github.com/juju/errors"

	"github.com/sirupsen/logrus"

	"github.com/adakailabs/gocard/config"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func Start(c *config.Config) {
	if err := c.CheckCardanoConfigFiles(); err != nil {
		logrus.Fatal(errors.ErrorStack(err))
	}

	if c.ContainerIsUP {
		logrus.Warn("container is already running")
		return
	}

	dockerImage := c.DockerImage

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
		c.ContainerConfig,
		c.HostConfig,
		nil, nil, "")
	if err != nil {
		panic(err)
	}

	if err = cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}

	// setup signal catching
	sigs := make(chan os.Signal, 1)

	// catch all signals since not explicitly listing
	signal.Notify(sigs)

	writeContainerID(resp.ID)
	readStartupLogsAndNotify(ctx, cli, resp.ID)
	containerWait(ctx, cli, resp.ID, sigs)
}

func containerWait(ctx context.Context, cli *client.Client, containerID string, sigs chan os.Signal) {
	statusCh, errCh := cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	for {
		select {
		case err := <-errCh:
			if err != nil {
				logrus.Error("container stoped with error: ", err.Error())
				err = os.Remove(config.GocardPidFile)
				if err != nil {
					logrus.Error("could not remove pid file")
				}
				logrus.Fatal("stopping now")
			}
		case this := <-statusCh:
			logrus.Info("container stoped with with status: ", this.StatusCode)
			err := os.Remove(config.GocardPidFile)
			if err != nil {
				logrus.Error("could not remove pid file")
			}
			logrus.Info("stopping now")
			os.Exit(1)

		case s := <-sigs:
			logrus.Tracef("RECEIVED SIGNAL: %s", s.String())
			if s.String() == "terminated" || s.String() == "interrupt" {
				logrus.Info("exiting with signal: ", s.String())
				stop(containerID)
				logrus.Exit(1)
			}
		}
		time.Sleep(time.Second)
	}
}

func systemDNofifyWatch() {
	logrus.Info("notifying readiness to systemd")
	_, err := daemon.SdNotify(false, daemon.SdNotifyReady)
	if err != nil {
		panic(err.Error())
	}

	go func() {
		interval, errs := daemon.SdWatchdogEnabled(false)
		if errs != nil || interval == 0 {
			return
		}
		for {
			_, err := daemon.SdNotify(false, daemon.SdNotifyWatchdog)
			if err != nil {
				panic(err.Error())
			}
			time.Sleep(interval / 3)
		}
	}()
}

func readStartupLogsAndNotify(ctx context.Context, cli *client.Client, containerID string) {
	timer := time.NewTicker(time.Second * 2)

	logMap := make(map[string]struct{})

	closure := func() {
		for range timer.C {
			out, errC := cli.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true})
			if errC != nil {
				panic(errC)
			}
			scanner := bufio.NewScanner(out)
			_, errC = cli.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true})
			if errC != nil {
				panic(errC)
			}
			for scanner.Scan() {
				logLine := scanner.Text()
				_, ok := logMap[logLine]
				if !ok {
					logrus.Info(logLine)
					logMap[logLine] = struct{}{}
				}
				if strings.Contains(logLine, "block replay progress (%) = 99") {
					logrus.Info("block replay complete")
					systemDNofifyWatch()
					return
				}
			}
		}
	}
	go closure()
}

func Stop(c *config.Config) {
	if c.ContainerIsUP {
		stop(c.ContainerID)
	}
}

func stop(containerID string) {
	_, err := daemon.SdNotify(false, daemon.SdNotifyStopping)
	if err != nil {
		panic(err.Error())
	}
	logrus.Info("attempting to stop container with ID: ", containerID)
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	if err = cli.ContainerStop(ctx, containerID, nil); err != nil {
		panic(err)
	}
	logrus.Info("stopped container")
	err = os.Remove(config.GocardPidFile)
	if err != nil {
		logrus.Error("could not remove pid file")
	}
}

func Init(c *config.Config) {
	c.SetCardanoInit()
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
