package rest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
)

// identityJavaCase 保存原版身份控制器的完整响应与请求后的身份服务状态。
type identityJavaCase struct {
	Name             string         `json:"name"`             // 场景名称
	Method           string         `json:"method"`           // HTTP 方法
	Path             string         `json:"path"`             // 资源路径
	RequestBody      string         `json:"requestBody"`      // 正文或上传标记
	Status           int            `json:"status"`           // 状态码
	ContentType      string         `json:"contentType"`      // 响应媒体类型
	ResponseBody     string         `json:"responseBody"`     // 完整正文
	ResponseBase64   string         `json:"responseBase64"`   // 二进制正文
	AfterUsers       map[string]any `json:"afterUsers"`       // 用户资料及密码检查结果
	AfterGroups      map[string]any `json:"afterGroups"`      // 组资料
	AfterInfo        map[string]any `json:"afterInfo"`        // 用户信息
	AfterPictures    map[string]any `json:"afterPictures"`    // 头像
	AfterMemberships map[string]any `json:"afterMemberships"` // 成员关系
}

// TestJavaIdentityRESTReference 对照十九个官方身份方法的完整对象、错误、二进制及真实级联副作用。
// @param t 测试上下文
// @return 无
func TestJavaIdentityRESTReference(t *testing.T) {
	data, err := os.ReadFile("../../testdata/compatibility/java/006-identity-rest.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Baseline string             `json:"baseline"`
		Cases    []identityJavaCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Baseline != "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e" || len(fixture.Cases) != 52 {
		t.Fatal("身份样本基线或场景数错误")
	}
	for _, scenario := range fixture.Cases {
		t.Run(scenario.Name, func(t *testing.T) {
			ctx := context.Background()
			app, err := engine.New(engine.Config{})
			if err != nil {
				t.Fatal(err)
			}
			for _, user := range []api.User{{ID: "alice", FirstName: "甲", LastName: "测试", DisplayName: "用户甲", Email: "alice@example.test", Password: "initial-reference"}, {ID: "bob", FirstName: "乙", LastName: "测试", DisplayName: "用户乙", Email: "bob@example.test", Password: "initial-reference"}} {
				if _, err := app.CreateUser(ctx, user); err != nil {
					t.Fatal(err)
				}
			}
			for _, group := range []api.Group{{ID: "review", Name: "审批组", Type: "assignment"}, {ID: "ops", Name: "运维组", Type: "security-role"}} {
				if _, err := app.CreateGroup(ctx, group); err != nil {
					t.Fatal(err)
				}
			}
			if err := app.AddMembership(ctx, "alice", "review"); err != nil {
				t.Fatal(err)
			}
			if err := app.SetUserInfo(ctx, "alice", "team", "原团队"); err != nil {
				t.Fatal(err)
			}
			if err := app.SetUserPicture(ctx, "alice", []byte{1, 2, 0, 255}, "image/png"); err != nil {
				t.Fatal(err)
			}
			handler := New(app, WithBasicAuth(map[string]string{"starter": "reference-secret"})).Handler()
			response := identityJavaRequest(t, handler, scenario)
			if response.Code != scenario.Status {
				t.Errorf("状态码%d，原版%d", response.Code, scenario.Status)
			}
			if scenario.ResponseBase64 != "" {
				if actual := base64.StdEncoding.EncodeToString(response.Body.Bytes()); actual != scenario.ResponseBase64 {
					t.Errorf("二进制正文%s，原版%s", actual, scenario.ResponseBase64)
				}
			} else if scenario.ResponseBody != "" {
				assertAsyncJSON(t, response.Body.Bytes(), []byte(scenario.ResponseBody))
			} else if strings.TrimSpace(response.Body.String()) != "" {
				t.Errorf("原版空正文被改为%s", response.Body.String())
			}
			actualType, actualParams, actualErr := mime.ParseMediaType(response.Header().Get("Content-Type"))
			expectedType, _, expectedErr := mime.ParseMediaType(scenario.ContentType)
			if scenario.ContentType != "" && (actualErr != nil || expectedErr != nil || actualType != expectedType || actualType == "application/json" && actualParams["charset"] != "" && !strings.EqualFold(actualParams["charset"], "utf-8")) {
				t.Errorf("媒体类型%s，原版%s", response.Header().Get("Content-Type"), scenario.ContentType)
			}
			// 由独立公共服务重新读取全部身份状态，不能只拼接原版正文而遗漏实际更新或删除。
			actual := identityJavaState(t, app)
			for key, want := range map[string]any{"users": scenario.AfterUsers, "groups": scenario.AfterGroups, "info": scenario.AfterInfo, "pictures": scenario.AfterPictures, "memberships": scenario.AfterMemberships} {
				gotData, _ := json.Marshal(actual[key])
				wantData, _ := json.Marshal(want)
				assertAsyncJSON(t, gotData, wantData)
			}
		})
	}
}

// identityJavaRequest 构造普通 JSON 或带真实文件媒体类型的头像请求。
// @param t 测试上下文
// @param handler 正式处理链
// @param scenario 原版场景
// @return 完整录制响应
func identityJavaRequest(t *testing.T, handler http.Handler, scenario identityJavaCase) *httptest.ResponseRecorder {
	t.Helper()
	if scenario.RequestBody != "multipart" {
		return runtimeJavaRequest(handler, "starter", scenario.Method, scenario.Path, scenario.RequestBody)
	}
	// 文件部分声明 PNG，并故意提供冲突的表单媒体类型，验证原版取值来源。
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="avatar.png"`}, "Content-Type": {"image/png"}}
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte{3, 4, 0, 255}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("mimeType", "image/gif"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(scenario.Method, "http://localhost"+scenario.Path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.SetBasicAuth("starter", "reference-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// identityJavaState 读取公共身份服务的资料、密码效果、信息、头像及组成员关系。
// @param t 测试上下文
// @param app 实际引擎
// @return 可与原版无损比较的业务状态
func identityJavaState(t *testing.T, app *engine.Engine) map[string]any {
	t.Helper()
	ctx := context.Background()
	users, err := app.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userState, info, pictures, memberships := map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{}
	// 资料与密码效果分别核对，不比较随机盐或依赖内部哈希编码。
	for _, user := range users {
		userState[user.ID] = map[string]any{"firstName": runtimeNullableString(user.FirstName), "lastName": runtimeNullableString(user.LastName), "displayName": runtimeNullableString(user.DisplayName), "email": runtimeNullableString(user.Email), "initialPasswordValid": app.CheckPassword(ctx, user.ID, "initial-reference") == nil, "updatedPasswordValid": app.CheckPassword(ctx, user.ID, "updated-reference") == nil}
		entries, err := app.GetUserInfoEntries(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		info[user.ID] = entries
		data, mime, err := app.GetUserPicture(ctx, user.ID)
		if err == nil {
			pictures[user.ID] = map[string]any{"mimeType": mime, "base64": base64.StdEncoding.EncodeToString(data)}
		} else if !errors.Is(err, api.ErrNotFound) {
			t.Fatal(err)
		}
		groups, err := app.ListUserGroups(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, group := range groups {
			ids = append(ids, group.ID)
		}
		memberships[user.ID] = ids
	}
	groups, err := app.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	groupState := map[string]any{}
	for _, group := range groups {
		groupState[group.ID] = map[string]any{"name": runtimeNullableString(group.Name), "type": runtimeNullableString(group.Type)}
	}
	return map[string]any{"users": userState, "groups": groupState, "info": info, "pictures": pictures, "memberships": memberships}
}
