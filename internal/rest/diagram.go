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
	s.writeInstanceDiagram(w, r, r.PathValue("id"), true)
}

// writeInstanceDiagram 按实例渲染并输出流程图（Accept 含 svg 输出 SVG，否则 PNG）。
// @param w 响应
// @param r 请求
// @param instanceID 流程实例 ID
// @param highlit true 时叠加当前活动实例高亮；false 输出普通流程图
func (s *Server) writeInstanceDiagram(w http.ResponseWriter, r *http.Request, instanceID string, highlit bool) {
	instance, err := s.engine.RuntimeService().GetProcessInstance(r.Context(), instanceID)
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
	highlights := map[string]bool{}
	// 高亮口径与 Java DefaultProcessDiagramGenerator 的 activeActivityIds 一致：当前活动实例集合
	if highlit {
		items, err := s.engine.RuntimeService().ListActivityInstances(r.Context(), instance.ID)
		if err != nil {
			writeError(w, err)
			return
		}
		for _, item := range items {
			highlights[item.ActivityID] = true
		}
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
