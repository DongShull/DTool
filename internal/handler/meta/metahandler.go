package meta

import (
	"net/http"

	"github.com/DongShull/DTool/internal/handler/handlerutil"
	"github.com/DongShull/DTool/internal/logic/meta"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
)

// MetaHandler 返回后端自描述元数据（版本 + 业务码枚举 + 默认文案），
// 供前端零硬编码渲染（GET /api/meta，与 /api/version 同级鉴权）。
func MetaHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlerutil.RunNoReq(w, r, func() (*types.Resp, error) {
			l := meta.NewMetaLogic(r.Context(), svcCtx)
			return l.Meta()
		})
	}
}
