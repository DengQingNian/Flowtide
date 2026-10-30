package rest

import (
	"net/http"
	"strings"

	"github.com/DengQingNian/Flowtide/internal/bpmn/diagram"
)

// handleDefinitionImage 返回流程定义图片。
// @param w 响应
// @param r 请求
func (s *Server) handleDefinitionImage(w http.ResponseWriter, r *http.Request) {
	definition, err := s.engine.RepositoryService().GetProcessDefinition(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	bpmnModel, err := s.engine.RepositoryService().GetBpmnModel(r.Context(), definition.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	xml, err := s.engine.RepositoryService().GetProcessDefinitionXML(r.Context(), definition.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	parsed, err := diagram.Parse(bpmnModel, xml)
	if err != nil {
		writeError(w, err)
		return
	}
	writeDiagram(w, r, parsed, nil)
}

// handleInstanceDiagram 返回流程实例图片。
// @param w 响应
// @param r 请求
func (s *Server) handleInstanceDiagram(w http.ResponseWriter, r *http.Request) {
	instance, err := s.engine.RuntimeService().GetProcessInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	definition, err := s.engine.RepositoryService().GetProcessDefinition(r.Context(), instance.ProcessDefinitionID)
	if err != nil {
		writeError(w, err)
		return
	}
	bpmnModel, err := s.engine.RepositoryService().GetBpmnModel(r.Context(), definition.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	xml, err := s.engine.RepositoryService().GetProcessDefinitionXML(r.Context(), definition.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	parsed, err := diagram.Parse(bpmnModel, xml)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := s.engine.RuntimeService().ListActivityInstances(r.Context(), instance.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	highlights := make(map[string]bool, len(items))
	for _, item := range items {
		highlights[item.ActivityID] = true
	}
	writeDiagram(w, r, parsed, highlights)
}

// writeDiagram 输出 PNG 或 SVG。
// @param w 响应
// @param r 请求
// @param parsed 流程图
// @param highlights 高亮活动
func writeDiagram(w http.ResponseWriter, r *http.Request, parsed *diagram.Diagram, highlights map[string]bool) {
	if strings.Contains(r.Header.Get("Accept"), "image/svg+xml") {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(diagram.RenderSVG(parsed, highlights))
		return
	}
	content, err := diagram.RenderPNG(parsed, highlights)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
