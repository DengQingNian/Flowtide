package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	// tlsClientAuthNone 不请求客户端证书，保持原有单向 TLS 行为。
	tlsClientAuthNone = "none"
	// tlsClientAuthRequest 请求客户端证书，但不强制要求。
	tlsClientAuthRequest = "request"
	// tlsClientAuthRequireAny 要求客户端证书，但不校验证书链。
	tlsClientAuthRequireAny = "require-any"
	// tlsClientAuthRequireVerify 要求客户端证书并校验证书链与用途。
	tlsClientAuthRequireVerify = "require-verify"
	// tlsVersion12 是默认的 TLS 最低版本。
	tlsVersion12 = "1.2"
	// tlsVersion13 是最高的 TLS 最低版本。
	tlsVersion13 = "1.3"
)

// buildTLSConfig 校验 TLS 参数并构造服务端 TLS 配置，服务端证书仍由 ListenAndServeTLS 加载。
// @param certFile 服务端证书路径，空值表示未启用 TLS
// @param keyFile 服务端私钥路径，必须与证书同时配置
// @param clientCAFile 客户端 CA 证书文件路径，PEM 格式可包含多个证书
// @param clientAuth 客户端证书认证模式，取值 none/request/require-any/require-verify
// @param minVersion TLS 最低版本，取值 1.2/1.3，空值默认 1.2
// @return 服务端 TLS 配置，未启用 TLS 时返回 nil
// @return 参数非法或 CA 解析失败时的错误
func buildTLSConfig(certFile, keyFile, clientCAFile, clientAuth, minVersion string) (*tls.Config, error) {
	authName := strings.ToLower(strings.TrimSpace(clientAuth))
	// 空字符串等价于默认值 none，避免命令行省略参数时被当成非法取值。
	if authName == "" {
		authName = tlsClientAuthNone
	}
	auth, err := parseClientAuth(authName)
	if err != nil {
		return nil, err
	}
	version, err := parseTLSMinVersion(minVersion)
	if err != nil {
		return nil, err
	}
	// 证书与私钥必须成对出现，只配置其中一项会静默回退为明文监听。
	if (certFile == "") != (keyFile == "") {
		return nil, errors.New("TLS 证书与私钥必须同时配置")
	}
	// 未启用 TLS 时客户端证书参数没有承载对象，必须保持默认值。
	if certFile == "" {
		return nil, validateDisabledTLS(clientCAFile, authName, auth)
	}
	// 只配置 CA 而未显式指定模式时，按“要求并校验客户端证书”处理，避免误配成单向 TLS。
	if auth == tls.NoClientCert && clientCAFile != "" {
		auth = tls.RequireAndVerifyClientCert
	}
	cfg := &tls.Config{MinVersion: version}
	// 不要求客户端证书时保持单向 TLS，CA 池留空。
	if auth == tls.NoClientCert {
		return cfg, nil
	}
	// 要求客户端证书却没有 CA 时无法完成校验，直接报错而不是启动后拒绝所有请求。
	if clientCAFile == "" {
		return nil, fmt.Errorf("-tls-client-auth=%s 需要配置 -tls-client-ca", authName)
	}
	pool, err := loadClientCAPool(clientCAFile)
	if err != nil {
		return nil, err
	}
	cfg.ClientCAs = pool
	cfg.ClientAuth = auth
	return cfg, nil
}

// validateDisabledTLS 校验未启用 TLS 时客户端证书参数必须为默认值。
// @param clientCAFile 客户端 CA 证书文件路径
// @param authName 规范化后的客户端认证模式
// @param auth 解析后的客户端认证类型
// @return 参数非法时的错误，合法时返回 nil
func validateDisabledTLS(clientCAFile, authName string, auth tls.ClientAuthType) error {
	// 未启用 TLS 时 CA 无法生效，静默忽略会掩盖配置错误。
	if clientCAFile != "" {
		return errors.New("-tls-client-ca 需要同时配置 -tls-cert 与 -tls-key")
	}
	// 未启用 TLS 时客户端认证模式同样无法生效。
	if auth != tls.NoClientCert {
		return fmt.Errorf("-tls-client-auth=%s 需要同时配置 -tls-cert 与 -tls-key", authName)
	}
	return nil
}

// parseClientAuth 将命令行取值转换为 TLS 客户端认证类型。
// @param name 已小写并去除空白后的认证模式
// @return 对应的认证类型
// @return 取值非法时的错误
func parseClientAuth(name string) (tls.ClientAuthType, error) {
	switch name {
	// 默认模式保持单向 TLS，不请求客户端证书。
	case tlsClientAuthNone:
		return tls.NoClientCert, nil
	// 请求证书但不强制，由业务层自行判断。
	case tlsClientAuthRequest:
		return tls.RequestClientCert, nil
	// 要求证书但不校验证书链，仅用于受控内网过渡。
	case tlsClientAuthRequireAny:
		return tls.RequireAnyClientCert, nil
	// 要求证书并校验证书链与客户端用途，是推荐的 mTLS 模式。
	case tlsClientAuthRequireVerify:
		return tls.RequireAndVerifyClientCert, nil
	default:
		return tls.NoClientCert, fmt.Errorf("非法的 -tls-client-auth 取值: %q，可选 none/request/require-any/require-verify", name)
	}
}

// parseTLSMinVersion 将命令行取值转换为 TLS 版本常量。
// @param value TLS 最低版本文本，空值默认 1.2
// @return 对应的 TLS 版本常量
// @return 取值非法时的错误
func parseTLSMinVersion(value string) (uint16, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	// 空值与 1.2 都使用 TLS 1.2，兼容原有行为。
	case "", tlsVersion12, "tls1.2":
		return tls.VersionTLS12, nil
	// 显式要求 TLS 1.3。
	case tlsVersion13, "tls1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("非法的 -tls-min-version 取值: %q，可选 1.2 或 1.3", value)
	}
}

// loadClientCAPool 读取 PEM 格式的客户端 CA 证书并构造证书池。
// @param path CA 证书文件路径
// @return 客户端 CA 证书池
// @return 读取或解析失败时的错误，错误信息只包含路径不包含证书内容
func loadClientCAPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取客户端 CA 证书失败: %w", err)
	}
	pool := x509.NewCertPool()
	// 文件中没有任何可解析证书时拒绝启动，避免启动后所有客户端握手失败。
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("客户端 CA 证书文件 %s 中没有有效的 PEM 证书", path)
	}
	return pool, nil
}

// validateClientCertIdentity 校验客户端证书身份映射的启动前提。
// 身份映射只能使用 TLS 层已验证的证书链，因此必须启用本地 TLS、配置客户端 CA 并要求校验。
// @param certFile 服务端证书路径
// @param clientCAFile 客户端 CA 证书路径
// @param clientAuth 客户端证书认证模式
// @return 配置不满足前提时的错误
func validateClientCertIdentity(certFile, clientCAFile, clientAuth string) error {
	// 未启用本地 TLS 时 r.TLS 为空，证书身份永远不会生效
	if certFile == "" {
		return errors.New("-tls-client-identity 需要同时配置 -tls-cert 与 -tls-key")
	}
	// 没有客户端 CA 就无法校验证书链，身份映射不可信
	if clientCAFile == "" {
		return errors.New("-tls-client-identity 需要配置 -tls-client-ca")
	}
	// require-verify 才能保证 VerifiedChains 非空；其他模式不会产生可映射身份
	if !strings.EqualFold(strings.TrimSpace(clientAuth), tlsClientAuthRequireVerify) {
		return fmt.Errorf("-tls-client-identity 需要 -tls-client-auth=%s", tlsClientAuthRequireVerify)
	}
	return nil
}
