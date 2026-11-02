package utiles

import (
	"context"
	"github.com/DongShull/DTool/internal/svc"
)

func RenameContainer(ctx *svc.ServiceContext, id string, newName string) error {
	unlockMutation := LockContainerMutation()
	defer unlockMutation()
	requestCtx, cancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer cancel()
	err := ctx.DockerClient.ContainerRename(requestCtx, id, newName)
	if err != nil {
		return err
	}
	return nil
}
