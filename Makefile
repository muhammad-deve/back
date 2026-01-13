CURRENT_DIR := $(shell pwd)
APP_DIR := $(CURRENT_DIR)/app
CMD_DIR := $(APP_DIR)/cmd
PB_DATA_DIR := $(APP_DIR)/artifacts/pb_data

PB_URL := http://127.0.0.1:8090
PB_SUPERUSER_EMAIL := muhammadjonxudaynazarov226@gmail.com
PB_SUPERUSER_PASSWORD :=1234567890
PB_CHANNELS_FILE := $(APP_DIR)/pkg/json/channels.json

PB_MOVIES_DIR := $(APP_DIR)/pkg/json
PB_MOVIES_WORKERS := 20
PB_MOVIES_RETRIES := 5
PB_MOVIES_MAX_ATTEMPTS := 50

REGISTRY=docker.io/oybekzdockerid
PROJECT_NAME=pocketbase-frst
ENV_TAG=dev
IMAGE_NAME=${PROJECT_NAME}-${ENV_TAG}
TAG=latest

build-image-dev:
	docker build --rm -t ${REGISTRY}/${IMAGE_NAME}:${TAG} .

push-image-dev:
	docker push ${REGISTRY}/${IMAGE_NAME}:${TAG}

run-app:
	cd ${CMD_DIR} && go run main.go serve --dir=${PB_DATA_DIR}

run-app-watch:
	cd $(APP_DIR) && nodemon --watch './**/*.go' --ignore 'app/artifacts/migrations/**' --signal SIGTERM --exec go run cmd/main.go serve --dir=${PB_DATA_DIR}

run:
	cd ${CMD_DIR} && go run main.go serve --dir=${PB_DATA_DIR}

channels:
	cd ${CMD_DIR} && go run main.go channels --dev=false --dir=${PB_DATA_DIR} --pb-url="${PB_URL}" --email="${PB_SUPERUSER_EMAIL}" --password="${PB_SUPERUSER_PASSWORD}" --file="${PB_CHANNELS_FILE}"

channel: channels

movies:
	cd ${CMD_DIR} && go run main.go movies --dev=false --dir=${PB_DATA_DIR} --pb-url="${PB_URL}" --email="${PB_SUPERUSER_EMAIL}" --password="${PB_SUPERUSER_PASSWORD}" --json-dir="${PB_MOVIES_DIR}" --workers=${PB_MOVIES_WORKERS} --retries=${PB_MOVIES_RETRIES} --max-attempts=${PB_MOVIES_MAX_ATTEMPTS}
