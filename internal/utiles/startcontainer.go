package utiles

import (
	"context"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/docker/docker/api/types/container"
)

func StartContainer(ctx *svc.ServiceContext, id string) error {
	unlockMutation := LockContainerMutation()
	defer unlockMutation()
	requestCtx, cancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer cancel()
	startOptions := container.StartOptions{}
	err := ctx.DockerClient.ContainerStart(requestCtx, id, startOptions)
	if err != nil {
		return err
	}

	return nil
}
