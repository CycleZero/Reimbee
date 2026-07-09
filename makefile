BINARY_NAME := app
BINARY_PATH := ./bin/${BINARY_NAME}
MAIN_FILE_DIR := ./

# wire 依赖注入代码生成
wire:
	wire ${MAIN_FILE_DIR}

# 编译前端
build-frontend:
	cd web && pnpm run build

# 编译（含前端）
build:
	cd web && pnpm run build
	go build -o ${BINARY_PATH} ${MAIN_FILE_DIR}

# 仅编译后端（开发时前端用 dev server）
build-backend:
	go build -o ${BINARY_PATH} ${MAIN_FILE_DIR}

# 一键重新构建（wire + build）
rebuild: wire build

# 编译至 Linux AMD64 平台（含前端）
build-linux:
	cd web && pnpm run build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ${BINARY_PATH} ${MAIN_FILE_DIR}

# 运行
run:
	go run .

# 安装依赖
tidy:
	go mod tidy

swagger:
	swag init