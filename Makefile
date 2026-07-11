DOCKER ?= docker
COMPOSE ?= docker compose

DOCKERHUB_USER ?=
IMAGE_NAME ?= armarium
TAG ?= latest

LOCAL_IMAGE := $(IMAGE_NAME):$(TAG)
REMOTE_IMAGE := $(DOCKERHUB_USER)/$(IMAGE_NAME):$(TAG)

.PHONY: help build up start down check-dockerhub-user tag login push

help:
	@echo "利用可能なターゲット:"
	@echo "  make build                $(LOCAL_IMAGE) をビルド"
	@echo "  make up                   Composeでビルドしてバックグラウンド起動"
	@echo "  make down                 Composeサービスを停止・削除"
	@echo "  make tag                  Docker Hub向けタグを付与"
	@echo "  make login                Docker Hubへログイン"
	@echo "  make push                 Docker Hubへpush"
	@echo ""
	@echo "変数は上書き可能です:"
	@echo "  make push DOCKERHUB_USER=example IMAGE_NAME=armarium TAG=v1.0.0"

build:
	$(DOCKER) build --tag $(LOCAL_IMAGE) .

up:
	ARMARIUM_IMAGE=$(IMAGE_NAME) ARMARIUM_TAG=$(TAG) $(COMPOSE) up --build --detach

start: up

down:
	$(COMPOSE) down

check-dockerhub-user:
	@test -n "$(DOCKERHUB_USER)" || (echo "エラー: DOCKERHUB_USERを指定してください。例: make push DOCKERHUB_USER=example" >&2; exit 1)

tag: check-dockerhub-user build
	$(DOCKER) tag $(LOCAL_IMAGE) $(REMOTE_IMAGE)

login: check-dockerhub-user
	$(DOCKER) login --username $(DOCKERHUB_USER)

push: tag
	$(DOCKER) push $(REMOTE_IMAGE)
