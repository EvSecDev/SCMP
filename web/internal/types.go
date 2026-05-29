// Package for private internals of the web backend server
package internal

import (
	"context"
	"encoding/json"
	"reflect"
)

type Catalog struct {
	Name               string
	Description        string
	Method             string
	Params             reflect.Type
	Result             reflect.Type
	AllowedPermissions []PermissionAction
	HandlerFunction    func(context.Context, context.Context, Request) (any, Error) // Split out base program context and the user context
}
type Request struct {
	JSONRPC string      `json:"jsonrpc"`          // Must be "2.0"
	Method  string      `json:"method"`           // Method name
	Params  interface{} `json:"params,omitempty"` // Optional parameters
	ID      string      `json:"id"`               // ID can be string, number, or null
}
type Response struct {
	JSONRPC string          `json:"jsonrpc"`          // Must be "2.0"
	Result  json.RawMessage `json:"result,omitempty"` // Only present if no error
	Error   *Error          `json:"error,omitempty"`  // Only present if error
	ID      string          `json:"id"`               // Must match the request ID
}
type Error struct {
	Code    int    `json:"code"`           // Error code
	Message string `json:"message"`        // Short description
	Data    string `json:"data,omitempty"` // Details
}

type WebConfig struct {
	UserCfg AuthConfig            `yaml:"userConfig"`
	RepoCfg map[string]RepoConfig `yaml:"repositories"`
	HTTP    HTTPConfig            `yaml:"http"`
}

type HTTPConfig struct {
	ListenPort   int    `yaml:"listenPort"`
	MaxReqPerSec int    `yaml:"rateLimitRPS"`
	TLSCertFile  string `yaml:"tlsCertFile"`
	TLSKeyFile   string `yaml:"tlsKeyFile"`
}

type RepoConfig struct {
	RootPath      string `yaml:"repositoryPath"`
	SSHConfigPath string `yaml:"sshConfigPath"`
}

type AuthConfig struct {
	JWTSecret   string       `yaml:"jwtSecret"`
	JWTValidSec int          `yaml:"jwtValidSec"`
	Users       []UserConfig `yaml:"users"`
}

type UserConfig struct {
	Username     string           `yaml:"username"`
	Email        string           `yaml:"email"`
	PasswordHash string           `yaml:"passwordHash"`
	Permissions  []UserPermission `yaml:"permissions"`
}

type PermissionAction string

type UserPermission struct {
	Repo    string             `yaml:"repository"`
	Actions []PermissionAction `yaml:"actions"`
}
