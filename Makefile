##
# 設定群
##
export GO111MODULE=on
DB_HOST:=localhost ##DBホストアドレス##
DB_PORT:=3306 ##DBポート番号##
DB_USER:=isucon ##DBユーザー##
DB_PASS:=isucon ##DBパス##
DB_NAME:=xsuportal ##DBネーム##
export DSTAT_MYSQL_USER=$(DB_USER)
export DSTAT_MYSQL_PWD=$(DB_PASS)
export DSTAT_MYSQL_HOST=$(DB_HOST)

MYSQL_CMD:=mysql -h$(DB_HOST) -P$(DB_PORT) -u$(DB_USER) -p$(DB_PASS) $(DB_NAME)

NGX_LOG:=/var/log/nginx/access.log
MYSQL_LOG:=/var/log/mysql/mysql-slow.log

KATARIBE_CFG:=./kataribe.toml

SLACKCAT:=slackcat --tee --channel ##チャンネル名##
SLACKRAW:=slackcat --channel ##チャンネル名##

PPROF:=go tool pprof -proto -output profile.pb.gz -seconds=120 http://localhost:6060/debug/pprof/profile
PPROF:=go tool pprof -proto -output fgprofile.pb.gz -seconds=120 http://localhost:6060/debug/fgprof

PROJECT_ROOT:=/home/isucon/webapp ##プロジェクトルートディレクトリ##
BUILD_DIR:=/home/isucon/webapp/golang ##バイナリ生成先##
WEB_BIN_NAME:=xsuportal ##生成バイナリ名##
API_BIN_NAME:=benchmark_server

CURL_OPTIONS:=-o /dev/null -s -w "%{http_code}\n"

APP_API_SERVICE:=xsuportal-api-golang.service ##systemdサービス名##
APP_WEB_SERVICE:=xsuportal-web-golang.service
REPOSITORY_URL:=git@github.com:FujishigeTemma/isucon10-final.git ##リポジトリのURL##

TAG:=0
HASH:=0

##
# コマンド群
##
all: build

.PHONY: push
push:
	@git add .
	@git commit -m "changes from server"
	@git push

.PHONY: update
update: pull build restart curl

pull:
	@git pull
	@cd $(BUILD_DIR) && \
	go mod download

.PHONY: build
build:
	@cd $(BUILD_DIR) && \
	go build -o $(BIN_NAME) && \
	go build -o $(BIN_NAME)

.PHONY: restart
restart:
	@sudo systemctl restart $(APP_SERVICE)

.PHONY: bench
bench: stash-log log

.PHONY: stash-log
stash-log:
	@$(eval when := $(shell date "+%s"))
	@mkdir -p ~/logs/$(when)
	@if [ -f $(NGX_LOG) ]; then \
		sudo mv -f $(NGX_LOG) ~/logs/$(when)/ ; \
	fi
	@if [ -f $(MYSQL_LOG) ]; then \
		sudo mv -f $(MYSQL_LOG) ~/logs/$(when)/ ; \
	fi
	@sudo systemctl restart nginx
	@sudo systemctl restart mysql

.PHONY: curl
curl:
	@curl localhost $(CURL_OPTIONS)

.PHONY: status
status:
	@sudo systemctl status $(APP_SERVICE)

.PHONY: rollback
rollback: reset build restart curl

reset:
ifeq ($(HASH),0)
	@echo "Please set variable: HASH={{commit_hash}}"
else
	@git reset --hard $(HASH)
endif

.PHONY: log
log:
	@sudo journalctl -u $(APP_SERVICE) -n10 -f

.PHONY: tag
tag:
ifeq ($(TAG),0)
	@echo "Please set variable: TAG={{bench_score}}"
else
	@git tag $(TAG)
	@git push origin $(TAG)
endif

.PHONY: dbstat
dbstat:
	dstat -T --mysql5-cmds --mysql5-io --mysql5-keys

.PHONY: analytics
analytics: kataru dumpslow digestslow

.PHONY: kataru
kataru:
	@sudo cat $(NGX_LOG) | kataribe -f $(KATARIBE_CFG) | $(SLACKCAT) kataribe

.PHONY: pprof
pprof:
	@$(PPROF)
	@go tool pprof -png -output pprof.png profile.pb.gz
	@$(SLACKRAW) pprof -n pprof.png ./pprof.png
	@go tool pprof -http=$(HOST_ADDRESS):6600 -no_browser profile.pb.gz

.PHONY: fgprof
fgprof:
	@$(FGPROF)
	@go tool pprof -png -output fgprof.png fgprofile.pb.gz
	@$(SLACKRAW) pprof -n fgprof.png ./fgprof.png
	@go tool pprof -http=$(HOST_ADDRESS):6600 -no_browser fgprofile.pb.gz

.PHONY: dumpslow
dumpslow:
	@sudo mysqldumpslow -s t -t 10 $(MYSQL_LOG) | $(SLACKCAT) slowquery

.PHONY: digestslow
digestslow: 
	@sudo pt-query-digest $(MYSQL_LOG) | $(SLACKCAT) slowquery

.PHONY: slow-on
slow-on:
	@sudo mysql -e "set global slow_query_log_file = '$(MYSQL_LOG)'; set global long_query_time = 0; set global slow_query_log = ON;"

.PHONY: slow-off
slow-off:
	@sudo mysql -e "set global slow_query_log = OFF;"

.PHONY: prune
prune: stash-log slow-off pull build curl
	@echo -e '\e[35mpprofをコードから取り除いたことを確認してください。\nNginxのログ出力を停止したことを確認してください。\e[0m\n'

##
# 諸々のインストールと設定
##
.PHONY: setup
setup: apt install-tools git-setup

apt:
	@sudo apt update
	@sudo apt upgrade -y

# バックアップするディレクトリは必要に応じて変更
backup:
	@tar -czvpf ~/backup.tar.gz -C ~/ .
	@echo -e '\e[35mscp {{ユーザー名}}@{{IP address}}:~/backup.tar.gz ~/Downloads/ を手元で実行してください。\nリカバリは他のサーバーからでも可能です。\e[0m\n'

install-go: 
	@wget https://golang.org/dl/go1.15.2.linux-amd64.tar.gz -O golang.tar.gz
	@tar -xzf golang.tar.gz
	@rm -rf golang.tar.gz
	@sudo mv go /usr/local/
	@sudo chmod +x /usr/local/go/bin/go
	@sudo ln -fs /usr/local/go/bin/go /usr/bin/go

install-tools: install-kataribe install-myprofiler install-slackcat
	@sudo apt install -y htop dstat percona-toolkit graphviz python3-mysqldb

# Nginxのログ解析
install-kataribe:
	@sudo apt install -y unzip
	@wget https://github.com/matsuu/kataribe/releases/download/v0.4.1/kataribe-v0.4.1_linux_amd64.zip -O kataribe.zip
	@sudo unzip -o -d /usr/local/bin/ kataribe.zip
	@sudo chmod +x /usr/local/bin/kataribe
	@rm kataribe.zip
	@cd | kataribe -generate
	@echo -e '\e[35mgenarated $(pwd)/kataribe.toml\nsudo nano /etc/nginx/nginx.conf 参考:https://github.com/matsuu/kataribe#Nginx\n##\n# Logging Settings\n##\n以下に追加してください。\n出力先には$(NGX_LOG)を指定してください。\nsudo nginx -t\nsudo systemctl reload nginx\e[0m\n'

install-myprofiler:
	@wget https://github.com/KLab/myprofiler/releases/download/0.2/myprofiler.linux_amd64.tar.gz -O myprofiler.tar.gz
	@tar -xzf myprofiler.tar.gz
	@rm myprofiler.tar.gz
	@sudo mv myprofiler /usr/local/bin/
	@sudo chmod +x /usr/local/bin/myprofiler
	@echo -e '\e[35mスロークエリの出力時には slow-on を実行してください。\e[0m\n'

install-slackcat:
	@wget https://github.com/bcicen/slackcat/releases/download/v1.6/slackcat-1.6-linux-amd64 -O slackcat
	@sudo mv slackcat /usr/local/bin/
	@sudo chmod +x /usr/local/bin/slackcat
	@slackcat --configure

# 必要があれば追加
ssh-key:
	@mkdir -p ~/.ssh
	@echo "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQD1g/cXhxb6VDjqgIfEeUwjSR6qf+n30Z2Cf4fhSX1ZF1x+Glqb/NsaRhEYqiG4jaLMXGZpXddQaUHn1eXdgM06BOVtDlDN2PeN5o6COfBnNR64Aa9+wYbEgmIXNW6ZBb9zKM2+n4rJE5Ihobqu68nJwUdmZv3BLeoP6Lr6Ze0N4PvCsLEwOsw9KqJuNybrAcGM/6DJuP7bZXTrQJp1Qwwxqdmk4dOEeWIdacQrq5W5nO4n2xXkmAAQ+Q78V/rwpq4SXdcxNzo3alzcZkOuvyZPXSY8xar7Vvb8ERXa6oFaCXkRdWr9VXXWmGpFDP0iXZE3/2yn3VeZEWRwvThoIFtd ryoha@ryoha-pc" >> ~/.ssh/authorized_keys
	@echo "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGVy1KogqLG7pPTcsm5zhC5RjddrAOfX7rHGK4K8y4s7 green@DESKTOP-V1DT07E" >> ~/.ssh/authorized_keys
	@echo "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKGvCNw4WJiTg327zw9AYchInFHxzlwBgzkm12fRIGAT tenma.x0@gmail.com" >> ~/.ssh/authorized_keys

git-setup:
	@git config --global user.email "tenma.x0@gmail.com"
	@git config --global user.name "FujishigeTemma"
	@ssh-keygen -t ed25519 -q
	@cat ~/.ssh/id_ed25519.pub
	@echo -e '\e[35mhttps://github.com/settings/keys に追加してください。\e[0m\n'

.PHONY: git-init
git-init:
	@git init
	@git add .
	@git commit -m "init"
	@git remote add origin $(REPOSITORY_URL)
	@git push -u origin master

