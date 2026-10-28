// Package mysql 提供 MySQL 开发配置、连接池和 Schema 初始化。
package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

// Config 是 MySQL 连接配置；禁止整体打印，以免泄露密码。
type Config struct {
	Host     string // 主机名或 IP
	Port     int    // TCP 端口
	User     string // 用户名
	Password string // 密码
	Database string // 数据库名
}

// LoadConfig 读取开发配置，进程环境变量优先于文件；不修改进程环境。
// @param path 配置文件路径，文件不存在时仅使用环境变量
// @return 数据库配置
// @return 不含文件内容的配置错误
func LoadConfig(path string) (Config, error) {
	values, err := godotenv.Read(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, errors.New("读取 MySQL 配置文件失败，请检查路径、权限与格式")
	}
	// 已存在的环境变量即使为空也优先，便于显式覆盖文件配置。
	get := func(key string) string {
		if value, exists := os.LookupEnv(key); exists {
			return value
		}
		return values[key]
	}
	port := 3306
	if value := get("db_mysql_port"); value != "" {
		port, err = strconv.Atoi(value)
		if err != nil {
			return Config{}, errors.New("db_mysql_port 必须为有效端口")
		}
	}
	cfg := Config{Host: strings.TrimSpace(get("db_mysql_host")), Port: port, User: get("db_mysql_user"), Password: get("db_mysql_pwd"), Database: strings.TrimSpace(get("db_mysql_db_name"))}
	return cfg, cfg.validate()
}

// validate 检查连接必填项及端口范围。
// @return 配置错误
func (c Config) validate() error {
	// 密码允许为空，以兼容本地开发实例；其余标识必须显式提供。
	if c.Host == "" || c.User == "" || c.Database == "" {
		return errors.New("MySQL 配置缺少 db_mysql_host、db_mysql_user 或 db_mysql_db_name")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("db_mysql_port 必须介于 1 和 65535")
	}
	return nil
}

// driverConfig 使用驱动配置对象构造连接，避免手工拼接密码和地址。
// @return 启用时间解析、UTC 和超时限制的驱动配置
func (c Config) driverConfig() *driver.Config {
	cfg := driver.NewConfig()
	cfg.Net, cfg.Addr = "tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	cfg.User, cfg.Passwd, cfg.DBName = c.User, c.Password, c.Database
	cfg.ParseTime, cfg.Loc = true, time.UTC
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 30*time.Second, 30*time.Second
	_ = cfg.Apply(driver.Charset("utf8mb4", ""))
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	return cfg
}

// Open 创建并验证连接池，失败时释放资源；调用方负责关闭成功返回的连接池。
// @param ctx 连接验证上下文
// @param cfg 数据库配置
// @return 可用连接池
// @return 配置或连接错误
func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	connector, err := driver.NewConnector(cfg.driverConfig())
	if err != nil {
		return nil, errors.New("创建 MySQL 驱动配置失败")
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetConnMaxIdleTime(time.Minute)
	// 连接验证失败时立即关闭连接池，防止启动失败遗留后台资源。
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接 MySQL 开发库失败: %w", err)
	}
	return db, nil
}
