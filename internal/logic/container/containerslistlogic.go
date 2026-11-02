package container

import (
	"context"
	"strings"
	"time"

	"github.com/DongShull/DTool/internal/utiles"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ContainersListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

type Info struct {
	Id          string `json:"id"`
	Status      string `json:"status"`
	Name        string `json:"name"`
	UsingImage  string `json:"usingImage"`
	CreateImage string `json:"createImage"`
	CreateTime  string `json:"createTime"`
	RunningTime string `json:"runningTime"`
	HaveUpdate  bool   `json:"haveUpdate"`
	IsCompose   bool   `json:"isCompose"`

	// 以下是前端渲染列表直接需要的字段（此前要再发 N 次详情请求才能拿到）：
	ImageName      string   `json:"imageName"`   // 不带 tag 的镜像名
	ImageTag       string   `json:"imageTag"`    // tag（<none> 表示无 tag）
	Ports          []string `json:"ports"`       // 形如 "0.0.0.0:13000→3000/tcp"
	Health         string   `json:"health"`      // healthy / unhealthy / starting；无健康检查为空
	StateDetail    string   `json:"stateDetail"` // running / exited / paused / created / restarting
	ComposeProject string   `json:"composeProject"`
	ComposeService string   `json:"composeService"`
	CreatedUnix    int64    `json:"createdUnix"`
	LabelCount     int      `json:"labelCount"`
}

func NewContainersListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ContainersListLogic {
	return &ContainersListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ContainersListLogic) ContainersList() (resp *types.Resp, err error) {
	// 获取所有容器（包括停止的容器）
	resp = &types.Resp{}
	list, err := utiles.GetContainerList(l.svcCtx)
	if err != nil {
		resp.Code = 500
		resp.Msg = err.Error()
		resp.Data = map[string]interface{}{}
		return resp, err
	}
	resp.Msg = "success"
	var containerInfoList []Info
	list = utiles.CheckImageUpdate(l.svcCtx, list)
	for _, v := range list {
		var containerInfo Info
		containerInfo.Id = v.ID
		containerInfo.Status = v.State
		if len(v.Names) > 0 {
			containerInfo.Name = strings.TrimPrefix(v.Names[0], "/")
		} else {
			containerInfo.Name = "获取容器名失败"
			l.Error("获取容器名失败: " + v.ID)
		}
		if v.Image != "" {
			containerInfo.UsingImage = v.Image
		} else {
			containerInfo.UsingImage = v.ImageID
			l.Error("容器镜像无名称: " + v.ID)
		}
		containerInspect, err := utiles.GetContainerInspect(l.svcCtx, v.ID)
		if err != nil || containerInspect.Config == nil {
			containerInfo.CreateImage = ""
			if err != nil {
				l.Error("获取容器创建镜像失败: " + v.ID + ": " + err.Error())
			} else {
				l.Error("容器配置为空: " + v.ID)
			}
		} else {
			containerInfo.CreateImage = containerInspect.Config.Image
		}
		t := time.Unix(v.Created, 0)
		containerInfo.CreateTime = t.Format("2006-01-02 15:04:05")
		containerInfo.RunningTime = v.Status
		containerInfo.HaveUpdate = v.Update
		// 由 docker compose 创建的容器带 project 标签，前端可提示优先走 compose 更新
		containerInfo.IsCompose = v.Labels["com.docker.compose.project"] != ""
		containerInfo.ComposeProject = v.Labels["com.docker.compose.project"]
		containerInfo.ComposeService = v.Labels["com.docker.compose.service"]
		containerInfo.LabelCount = len(v.Labels)
		containerInfo.CreatedUnix = v.Created
		containerInfo.StateDetail = v.State
		containerInfo.Ports = utiles.FormatPorts(v.Ports)
		containerInfo.ImageName, containerInfo.ImageTag = utiles.SplitImageRef(v.Image)
		// 健康状态只在 list 响应里存在字符串形式，取不到时留空。
		containerInfo.Health = utiles.HealthFromStatus(v.Status)
		containerInfoList = append(containerInfoList, containerInfo)
	}
	resp.Data = containerInfoList
	return resp, nil
}
