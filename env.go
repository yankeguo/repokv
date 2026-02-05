package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

var (
	ADMIN_API_KEY    string = "admin-api-key"
	DATA_DIR         string = "./data"
	CONF_DIR         string = "./conf"
	REPO_MAX_RETRIES int    = 3
)

func envStr(key string, out *string) {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		*out = v
	}
}

func envInt(key string, out *int) {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*out = n
		}
	}
}

func init() {
	_ = godotenv.Load()

	envStr("REPOKV_ADMIN_API_KEY", &ADMIN_API_KEY)
	envStr("REPOKV_DATA_DIR", &DATA_DIR)
	envStr("REPOKV_CONF_DIR", &CONF_DIR)
	envInt("REPOKV_REPO_MAX_RETRIES", &REPO_MAX_RETRIES)
}
