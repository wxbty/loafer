package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"loafer-agent/internal/config"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
)

// FileHandler 对应原 Java FileController，处理文件浏览、读写、上传与下载。
type FileHandler struct {
	cfg *config.AppConfig
}

// NewFileHandler 构造文件处理器。
func NewFileHandler(cfg *config.AppConfig) *FileHandler {
	return &FileHandler{cfg: cfg}
}

// RegisterRoutes 注册文件相关路由（/files）。
func (h *FileHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/files")
	{
		g.GET("/list", h.ListDirectory)
		g.GET("/content", h.ReadFileContent)
		g.PUT("/content", h.SaveFileContent)
		g.GET("/tail", h.ReadLogTail)
		g.GET("/download", h.DownloadFile)
		g.GET("/image-preview", h.ImagePreview)
		g.GET("/audio", h.AudioPreview)
		g.DELETE("/delete", h.DeletePath)
		g.POST("/data-dir/mkdir", h.CreateDataSubDirectory)
		g.DELETE("/data-dir/delete", h.DeleteDataSubDirectory)
		g.PUT("/data-dir/rename", h.RenameDataSubDirectory)
		g.POST("/data-dir/upload", h.UploadDataDirFile)
		g.POST("/temp-image/upload", h.UploadTempImage)
	}
}

// FileInfo 文件信息结构。
type FileInfo struct {
	Name      string      `json:"name"`
	Path      string      `json:"path"`
	IsDir     bool        `json:"isDirectory"`
	Size      int64       `json:"size"`
	ModTime   string      `json:"modTime"`
	Children  []FileInfo  `json:"children,omitempty"`
}

// ListDirectory 对应 GET /files/list，列出目录内容。
func (h *FileHandler) ListDirectory(c *gin.Context) {
	dirPath := c.Query("path")
	if dirPath == "" {
		util.Fail(c, http.StatusBadRequest, "path 参数不能为空")
		return
	}

	info, err := os.Stat(dirPath)
	if err != nil {
		// 目录不存在时返回空列表，而非 500 错误（新项目工作目录可能尚未创建）
		util.OK(c, []FileInfo{})
		return
	}

	if !info.IsDir() {
		// 单个文件
		util.OKWithData(c, FileInfo{
			Name:    info.Name(),
			Path:    dirPath,
			IsDir:   false,
			Size:    info.Size(),
		})
		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		util.Fail500(c, "读取目录失败: "+err.Error())
		return
	}

	var files []FileInfo
	for _, entry := range entries {
		// 跳过隐藏文件
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    filepath.Join(dirPath, entry.Name()),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
		})
	}

	// 目录排前面，然后按名称排序
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})

	util.OKWithData(c, gin.H{
		"name":    info.Name(),
		"path":    dirPath,
		"isDir":    true, // 保留兼容；前端实际读取 isDirectory
		"isDirectory": true,
		"children": files,
	})
}

// ReadFileContent 对应 GET /files/content，读取文件文本内容。
func (h *FileHandler) ReadFileContent(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		util.Fail(c, http.StatusBadRequest, "path 参数不能为空")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		util.Fail500(c, "读取文件失败: "+err.Error())
		return
	}
	util.OKWithData(c, gin.H{
		"path":    path,
		"content": string(data),
	})
}

// SaveFileContent 对应 PUT /files/content，保存文件文本内容。
func (h *FileHandler) SaveFileContent(c *gin.Context) {
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if body.Path == "" {
		util.Fail(c, http.StatusBadRequest, "path 不能为空")
		return
	}
	if err := os.WriteFile(body.Path, []byte(body.Content), 0644); err != nil {
		util.Fail500(c, "写入文件失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// ReadLogTail 对应 GET /files/tail，读取日志文件末尾 N 行。
func (h *FileHandler) ReadLogTail(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		util.Fail(c, http.StatusBadRequest, "path 参数不能为空")
		return
	}
	lineCount := 200
	if n := c.Query("lineCount"); n != "" {
		if v, err := parseIntDefault(n, 200); err == nil {
			lineCount = v
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		util.Fail500(c, "读取文件失败: "+err.Error())
		return
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > lineCount {
		lines = lines[len(lines)-lineCount:]
	}
	util.OKWithData(c, gin.H{
		"path":    path,
		"content": strings.Join(lines, "\n"),
	})
}

// DownloadFile 对应 GET /files/download，下载文件。
func (h *FileHandler) DownloadFile(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		util.Fail(c, http.StatusBadRequest, "path 参数不能为空")
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		util.Fail500(c, "文件不存在: "+err.Error())
		return
	}
	if info.IsDir() {
		util.Fail(c, http.StatusBadRequest, "不能下载目录")
		return
	}
	c.FileAttachment(path, info.Name())
}

// ImagePreview 对应 GET /files/image-preview，图片预览（无需认证）。
func (h *FileHandler) ImagePreview(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		util.Fail(c, http.StatusBadRequest, "path 参数不能为空")
		return
	}
	c.File(path)
}

// AudioPreview 对应 GET /files/audio，音频预览（无需认证）。
func (h *FileHandler) AudioPreview(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		util.Fail(c, http.StatusBadRequest, "path 参数不能为空")
		return
	}
	c.File(path)
}

// DeletePath 对应 DELETE /files/delete，删除文件或目录。
func (h *FileHandler) DeletePath(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		util.Fail(c, http.StatusBadRequest, "path 参数不能为空")
		return
	}
	if err := os.RemoveAll(path); err != nil {
		util.Fail500(c, "删除失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// CreateDataSubDirectory 对应 POST /files/data-dir/mkdir，创建子目录。
func (h *FileHandler) CreateDataSubDirectory(c *gin.Context) {
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	dirPath := filepath.Join(body.Path, body.Name)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		util.Fail500(c, "创建目录失败: "+err.Error())
		return
	}
	util.OKWithData(c, gin.H{"path": dirPath})
}

// DeleteDataSubDirectory 对应 DELETE /files/data-dir/delete，删除数据子目录。
func (h *FileHandler) DeleteDataSubDirectory(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		util.Fail(c, http.StatusBadRequest, "path 参数不能为空")
		return
	}
	if err := os.RemoveAll(path); err != nil {
		util.Fail500(c, "删除失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// RenameDataSubDirectory 对应 PUT /files/data-dir/rename，重命名目录。
func (h *FileHandler) RenameDataSubDirectory(c *gin.Context) {
	var body struct {
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if err := os.Rename(body.OldPath, body.NewPath); err != nil {
		util.Fail500(c, "重命名失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// UploadDataDirFile 对应 POST /files/data-dir/upload，上传文件到数据目录。
func (h *FileHandler) UploadDataDirFile(c *gin.Context) {
	targetDir := c.PostForm("targetDir")
	if targetDir == "" {
		util.Fail(c, http.StatusBadRequest, "targetDir 不能为空")
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		util.Fail(c, http.StatusBadRequest, "上传文件不能为空")
		return
	}
	dstPath := filepath.Join(targetDir, file.Filename)
	if err := c.SaveUploadedFile(file, dstPath); err != nil {
		util.Fail500(c, "保存文件失败: "+err.Error())
		return
	}
	util.OKWithData(c, gin.H{
		"path": dstPath,
		"name": file.Filename,
		"size": file.Size,
	})
}

// UploadTempImage 对应 POST /files/temp-image/upload，上传临时图片。
func (h *FileHandler) UploadTempImage(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		util.Fail(c, http.StatusBadRequest, "上传文件不能为空")
		return
	}
	tempDir := filepath.Join(os.TempDir(), "loafer-images")
	os.MkdirAll(tempDir, 0755)
	dstPath := filepath.Join(tempDir, file.Filename)
	if err := c.SaveUploadedFile(file, dstPath); err != nil {
		util.Fail500(c, "保存文件失败: "+err.Error())
		return
	}
	util.OKWithData(c, gin.H{
		"path": dstPath,
		"name": file.Filename,
		"size": file.Size,
	})
}

// parseIntDefault 解析整数，失败返回默认值。
func parseIntDefault(s string, def int) (int, error) {
	n := 0
	neg := false
	if len(s) == 0 {
		return def, nil
	}
	i := 0
	if s[0] == '-' {
		neg = true
		i = 1
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return def, nil
		}
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}

