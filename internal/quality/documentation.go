package quality

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

// CheckDocumentation 校验所有命名函数的中文说明、参数与返回值注释。
// @param root 仓库根目录
// @return 违规清单
// @return 文件解析错误
func CheckDocumentation(root string) ([]string, error) {
	failures := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == "tmp" || entry.Name() == "temp" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			// 每个函数逐项校验，报告文件和行号，便于直接定位违规。
			text := ""
			if function.Doc != nil {
				text = function.Doc.Text()
			}
			reason := ""
			if !strings.HasPrefix(text, function.Name.Name+" ") || !regexp.MustCompile(`\p{Han}`).MatchString(text) {
				reason = "缺少以函数名开头的中文说明"
			}
			for _, parameter := range function.Type.Params.List {
				for _, name := range parameter.Names {
					if !regexp.MustCompile(`(?m)^@param\s+` + regexp.QuoteMeta(name.Name) + `(?:\s|$)`).MatchString(text) {
						reason = "缺少参数注释: " + name.Name
					}
				}
			}
			returns := 0
			if function.Type.Results != nil {
				for _, result := range function.Type.Results.List {
					count := len(result.Names)
					if count == 0 {
						count = 1
					}
					returns += count
				}
			}
			if strings.Count(text, "@return") < returns {
				reason = "返回值注释数量不足"
			}
			if reason != "" {
				failures = append(failures, fmt.Sprintf("%s %s: %s", fileSet.Position(function.Pos()), function.Name.Name, reason))
			}
		}
		return nil
	})
	return failures, err
}
