# 使用 Alpine 作为基础镜像
FROM alpine
LABEL authors="onlyLTY, DongShull"
WORKDIR /app

ARG TARGETPLATFORM

COPY dt-back/dist/$TARGETPLATFORM/DTool ./DTool
COPY dt-back/etc/. ./etc
COPY start.sh ./start.sh

RUN apk add --no-cache tzdata \
    && chmod +x start.sh \
    && chmod +x DTool

# 设置环境变量
ENV secretKey="" \
    DOCKER_HOST="unix:///var/run/docker.sock" \
    TZ="Asia/Shanghai"

VOLUME ["/data"]

# 健康检查：主页可访问即视为健康
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:12712/ || exit 1

# 设置启动命令
CMD ["./start.sh"]
