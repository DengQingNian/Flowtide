package quality

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CheckBaseline 校验参考版本清单的完整性及已锁定制品摘要。
// @param root 仓库根目录
// @return 版本或制品错误
func CheckBaseline(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "testdata/compatibility/baseline.json"))
	if err != nil {
		return err
	}
	var manifest struct {
		Version       int    `json:"version"`
		JavaCommit    string `json:"java_commit"`
		SchemaVersion string `json:"schema_version"`
		Artifacts     []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	commit, err := hex.DecodeString(manifest.JavaCommit)
	if err != nil || len(commit) != 20 || manifest.Version != 1 || manifest.SchemaVersion == "" || len(manifest.Artifacts) == 0 {
		return fmt.Errorf("参考版本清单无效")
	}
	for _, artifact := range manifest.Artifacts {
		// 清单只能引用仓库内的相对路径，禁止检查越界文件。
		if !filepath.IsLocal(artifact.Path) {
			return fmt.Errorf("制品路径必须位于仓库内")
		}
		data, err := os.ReadFile(filepath.Join(root, artifact.Path))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != artifact.SHA256 {
			return fmt.Errorf("制品摘要发生变化: %s", artifact.Path)
		}
	}
	return nil
}
