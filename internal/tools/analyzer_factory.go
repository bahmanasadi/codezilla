package tools

import (
	"codezilla/pkg/logger"
)

// AnalyzerFactory creates file analyzers based on configuration
type AnalyzerFactory struct {
	llmClient LLMClient
	logger    *logger.Logger
}

// NewAnalyzerFactory creates a new analyzer factory
func NewAnalyzerFactory(llmClient LLMClient, logger *logger.Logger) *AnalyzerFactory {
	return &AnalyzerFactory{
		llmClient: llmClient,
		logger:    logger,
	}
}

// CreateAnalyzer creates an appropriate analyzer based on availability
func (f *AnalyzerFactory) CreateAnalyzer(useLLM bool) FileAnalyzer {
	if useLLM && f.llmClient != nil {
		return NewLLMFileAnalyzer(f.llmClient, f.logger)
	}
	return NewDefaultFileAnalyzer()
}

// CreateProjectScanAnalyzer creates a project scan analyzer tool
func (f *AnalyzerFactory) CreateProjectScanAnalyzer(useLLM bool) *ProjectScanAnalyzerTool {
	analyzer := f.CreateAnalyzer(useLLM)
	return NewProjectScanAnalyzerTool(analyzer)
}
