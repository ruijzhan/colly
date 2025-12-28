.PHONY: all server clean

OUTPUT_DIR := _output/bin
SERVER_BIN := $(OUTPUT_DIR)/server

all: server

server:
	@mkdir -p $(OUTPUT_DIR)
	go build -o $(SERVER_BIN) ./cmd/server

clean:
	@rm -rf _output
