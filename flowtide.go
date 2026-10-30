// Package flowtide 提供可从独立 Go 模块构造的公共引擎入口。
package flowtide

import "github.com/DengQingNian/Flowtide/internal/engine"

// Config 是引擎配置，默认使用内存状态与审计历史。
type Config = engine.Config

// Engine 是公共引擎实现，通过 api 中的服务接口执行业务操作。
type Engine = engine.Engine

// Delegate 是可注册的服务任务委托。
type Delegate = engine.Delegate

// Execution 是服务任务的执行上下文。
type Execution = engine.Execution

// New 构造公共引擎。
// @param cfg 引擎配置
// @return 引擎
// @return 配置或恢复错误
func New(cfg Config) (*Engine, error) { return engine.New(cfg) }
