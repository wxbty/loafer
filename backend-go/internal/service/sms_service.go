package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"loafer-agent/internal/config"
	"loafer-agent/internal/model"

	"gorm.io/gorm"
)

const (
	smsServiceName = "sms"
	smsRegion      = "cn-north-1"
	smsHost        = "sms.volcengineapi.com"
	smsEndpoint    = "https://sms.volcengineapi.com"
)

// SmsService 火山引擎短信服务，封装短信发送与项目级短信配置管理。
// 使用火山引擎签名 V4 算法（HMAC-SHA256）进行 API 认证。
type SmsService struct {
	db  *gorm.DB
	cfg *config.Config
}

// NewSmsService 构造短信服务。
func NewSmsService(db *gorm.DB, cfg *config.Config) *SmsService {
	return &SmsService{db: db, cfg: cfg}
}

// volcanoSmsRequest 火山短信 SendSms API 请求体。
type volcanoSmsRequest struct {
	SmsAccount    string `json:"SmsAccount"`
	Sign          string `json:"Sign"`
	TemplateID    string `json:"TemplateID"`
	TemplateParam string `json:"TemplateParam"`
	PhoneNumbers  string `json:"PhoneNumbers"`
}

// volcanoSmsResponse 火山短信 SendSms API 响应体。
type volcanoSmsResponse struct {
	ResponseMetadata struct {
		RequestID string `json:"RequestId"`
		Action   string `json:"Action"`
		Version  string `json:"Version"`
		Service  string `json:"Service"`
		Region   string `json:"Region"`
		Error    *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"ResponseMetadata"`
	Result struct {
		MessageID []string `json:"MessageID"`
	} `json:"Result"`
}

// SendSMS 通过火山引擎短信 API 向指定手机号发送短信。
// templateParams 为模板变量键值对，例如 {"code": "1234"}。
// 使用全局配置的短信凭证，采用签名 V4 算法进行认证。
func (s *SmsService) SendSMS(phoneNumbers []string, templateParams map[string]string) error {
	if len(phoneNumbers) == 0 {
		return errors.New("手机号列表不能为空")
	}

	accessKey := s.cfg.SMS.AccessKey
	secretKey := s.cfg.SMS.SecretKey
	accountID := s.cfg.SMS.AccountID
	signName := s.cfg.SMS.SignName
	templateID := s.cfg.SMS.TemplateID

	if accessKey == "" || secretKey == "" {
		return errors.New("短信服务凭证未配置（AccessKey/SecretKey 为空）")
	}

	return s.doSendSMS(phoneNumbers, templateParams, accessKey, secretKey, accountID, signName, templateID)
}

// SendProjectNotification 发送项目状态通知短信。
// 模板参数为 {"project_name": projectName, "status": status}。
// 优先使用项目级短信配置，若项目未配置或未启用则回退到全局配置。
func (s *SmsService) SendProjectNotification(projectID int64, phoneNumbers []string, projectName, status string) error {
	if len(phoneNumbers) == 0 {
		return errors.New("手机号列表不能为空")
	}

	templateParams := map[string]string{
		"project_name": projectName,
		"status":       status,
	}

	projCfg, err := s.GetProjectSmsConfig(projectID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("查询项目短信配置失败: %w", err)
	}

	// 项目级配置存在且已启用时使用项目凭证
	if projCfg != nil && projCfg.Enabled == 1 {
		log.Printf("使用项目 %d 独立短信配置发送通知", projectID)
		return s.doSendSMS(phoneNumbers, templateParams,
			projCfg.AccessKey, projCfg.SecretKey, projCfg.AccountID, projCfg.SignName, projCfg.TemplateID)
	}

	// 回退到全局配置
	log.Printf("项目 %d 未配置独立短信服务或未启用，回退到全局配置", projectID)
	return s.SendSMS(phoneNumbers, templateParams)
}

// SaveProjectSmsConfig 保存项目级短信配置。若已存在则更新，否则新建。
func (s *SmsService) SaveProjectSmsConfig(projectID int64, cfg model.SmsConfig) error {
	cfg.ProjectID = projectID

	var existing model.SmsConfig
	err := s.db.Where("project_id = ?", projectID).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := s.db.Create(&cfg).Error; err != nil {
				return fmt.Errorf("保存项目短信配置失败: %w", err)
			}
			return nil
		}
		return fmt.Errorf("查询项目短信配置失败: %w", err)
	}

	// 更新已有记录（不覆盖 created_at：cfg 为入参新结构体，CreatedAt 为零值，
	// 全量写入会触发 MySQL 严格模式 Error 1292）
	cfg.ID = existing.ID
	if err := s.db.Model(&cfg).Select("*").Omit("created_at").Updates(&cfg).Error; err != nil {
		return fmt.Errorf("更新项目短信配置失败: %w", err)
	}
	return nil
}

// GetProjectSmsConfig 获取项目级短信配置。未找到时返回 gorm.ErrRecordNotFound。
func (s *SmsService) GetProjectSmsConfig(projectID int64) (*model.SmsConfig, error) {
	var cfg model.SmsConfig
	if err := s.db.Where("project_id = ?", projectID).First(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

// doSendSMS 执行实际的短信发送逻辑：构建请求体、签名、发送并校验响应。
func (s *SmsService) doSendSMS(phoneNumbers []string, templateParams map[string]string,
	accessKey, secretKey, accountID, signName, templateID string) error {

	// 序列化模板参数为 JSON 字符串
	paramJSON, err := json.Marshal(templateParams)
	if err != nil {
		return fmt.Errorf("序列化模板参数失败: %w", err)
	}

	reqBody := volcanoSmsRequest{
		SmsAccount:    accountID,
		Sign:          signName,
		TemplateID:     templateID,
		TemplateParam: string(paramJSON),
		PhoneNumbers:  strings.Join(phoneNumbers, ","),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求体失败: %w", err)
	}

	requestURL := fmt.Sprintf("%s/?Action=SendSms&Version=2020-01-01", smsEndpoint)
	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 火山引擎签名 V4
	s.signRequest(req, bodyBytes, accessKey, secretKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送短信请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	var smsResp volcanoSmsResponse
	if err := json.Unmarshal(respBody, &smsResp); err != nil {
		return fmt.Errorf("解析响应失败: %w, body: %s", err, string(respBody))
	}

	if smsResp.ResponseMetadata.Error != nil {
		return fmt.Errorf("短信发送失败: code=%s, message=%s",
			smsResp.ResponseMetadata.Error.Code, smsResp.ResponseMetadata.Error.Message)
	}

	log.Printf("短信发送成功, MessageID: %v", smsResp.Result.MessageID)
	return nil
}

// signRequest 构建火山引擎签名 V4 并将签名信息写入请求头。
// 签名流程：构建规范化请求 -> 构建待签名字符串 -> 派生签名密钥 -> 计算 HMAC-SHA256 签名。
func (s *SmsService) signRequest(req *http.Request, body []byte, accessKey, secretKey string) {
	now := time.Now().UTC()
	xDate := now.Format("20060102T150405Z")
	shortDate := now.Format("20060102")

	// --- 1. 构建规范化请求 (Canonical Request) ---
	// 查询参数按字典序排列：Action < Version
	canonicalQueryString := "Action=SendSms&Version=2020-01-01"
	// 请求头按字典序排列：content-type < host < x-date
	contentType := req.Header.Get("Content-Type")
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-date:%s\n",
		strings.ToLower(contentType), smsHost, xDate)
	signedHeaders := "content-type;host;x-date"

	hashedPayload := sha256Hex(body)

	canonicalRequest := strings.Join([]string{
		"POST",
		"/",
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")

	// --- 2. 构建待签名字符串 (String to Sign) ---
	credentialScope := fmt.Sprintf("%s/%s/%s/request", shortDate, smsRegion, smsServiceName)
	hashedCanonicalRequest := sha256Hex([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"HMAC-SHA256",
		xDate,
		credentialScope,
		hashedCanonicalRequest,
	}, "\n")

	// --- 3. 派生签名密钥 (Signing Key) ---
	// kDate = HMAC(secretKey, date)
	// kRegion = HMAC(kDate, region)
	// kService = HMAC(kRegion, service)
	// kSigning = HMAC(kService, "request")
	kDate := hmacSHA256([]byte(secretKey), []byte(shortDate))
	kRegion := hmacSHA256(kDate, []byte(smsRegion))
	kService := hmacSHA256(kRegion, []byte(smsServiceName))
	kSigning := hmacSHA256(kService, []byte("request"))

	// --- 4. 计算签名 ---
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	// --- 5. 写入 Authorization 头 ---
	authorization := fmt.Sprintf("HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)

	req.Header.Set("Host", smsHost)
	req.Header.Set("X-Date", xDate)
	req.Header.Set("Authorization", authorization)
}

// sha256Hex 计算数据的 SHA-256 哈希并返回十六进制字符串。
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// hmacSHA256 使用密钥对数据进行 HMAC-SHA256 运算并返回字节切片。
func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
