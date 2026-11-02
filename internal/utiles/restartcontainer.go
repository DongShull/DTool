package utiles

import (
	"context"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/docker/docker/api/types/container"
)

func RestartContainer(ctx *svc.ServiceContext, id string) error {
	timeout := 10
	signal := "SIGTERM"
	stopOptions := container.StopOptions{
		Signal:  signal,
		Timeout: &timeout,
	}
	unlockMutation := LockContainerMutation()
	defer unlockMutation()
	requestCtx, cancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer cancel()
	err := ctx.DockerClient.ContainerRestart(requestCtx, id, stopOptions)
	if err != nil {
		return err
	}
	return nil
}
