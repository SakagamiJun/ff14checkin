package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

func getSdoAppClient() tls_client.HttpClient {
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(15),
		tls_client.WithClientProfile(profiles.Chrome_124),
	}
	client, _ := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	return client
}

func buildSdoAppURL(endpoint string, params map[string]string, cfg *Config) string {
	if cfg.DeviceId == "" {
		cfg.DeviceId = generateUUID()
		SaveConfig(configFile, cfg)
	}
	if cfg.MacId == "" {
		cfg.MacId = generateUUID()
		SaveConfig(configFile, cfg)
	}

	q := url.Values{}
	q.Set("authenSource", "1")
	q.Set("appId", "100001900")
	q.Set("areaId", "1")
	q.Set("appIdSite", "100001900")
	q.Set("locale", "zh_CN")
	q.Set("productId", "4")
	q.Set("frameType", "1")
	q.Set("endpointOS", "1")
	q.Set("version", "21")
	q.Set("customSecurityLevel", "2")
	q.Set("deviceId", cfg.DeviceId)
	q.Set("thirdLoginExtern", "0")
	q.Set("macId", cfg.MacId)
	q.Set("epIp", "")
	q.Set("epName", "ff14checkin")
	q.Set("extendInfo", "")
	q.Set("sdoVersion", "")
	q.Set("runTimeId", "")
	q.Set("productVersion", "1.9.7.10")
	q.Set("tag", "0")

	for k, v := range params {
		q.Set(k, v)
	}

	return "https://cas.sdo.com/authen/" + endpoint + "?" + q.Encode()
}

func doSdoAppRequest(endpoint string, params map[string]string, cfg *Config, tgt string) (map[string]interface{}, error) {
	reqURL := buildSdoAppURL(endpoint, params, cfg)
	req, _ := http.NewRequest("GET", reqURL, nil)

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "Mozilla/4.0 (compatible; MSIE 8.0; Windows NT 5.1; Trident/4.0)")
	req.Header.Set("Host", "cas.sdo.com")

	if tgt != "" {
		req.Header.Set("Cookie", fmt.Sprintf("CASTGC=%s; CAS_LOGIN_STATE=1", tgt))
	} else {
		macHash := md5.Sum([]byte(cfg.MacId))
		cid := "CID" + strings.ToUpper(hex.EncodeToString(macHash[:]))
		req.Header.Set("Cookie", fmt.Sprintf("CASCID=%s; SECURE_CASCID=%s;", cid, cid))
	}

	client := getSdoAppClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("json parse error: %v, body: %s", err, string(body))
	}

	return result, nil
}

func getSdoGuid(cfg *Config) (string, error) {
	params := map[string]string{"generateDynamicKey": "1"}
	res, err := doSdoAppRequest("getGuid.json", params, cfg, "")
	if err != nil {
		return "", err
	}
	if data, ok := res["data"].(map[string]interface{}); ok {
		if guid, ok := data["guid"].(string); ok {
			return guid, nil
		}
	}
	return "", fmt.Errorf("failed to get guid: %v", res)
}

func upgradeToAppSession(cfg *Config) {
	// Find CASTGC from current cookies
	var tgt string
	for _, c := range cfg.Cookies {
		if c.Name == "CASTGC" {
			tgt = c.Value
			break
		}
	}
	if tgt == "" {
		log.Println("未找到 CASTGC，无法升级为长效 App Session")
		return
	}

	log.Println("正在尝试将 Web Session 升级为 30天长效 App Session...")

	// 1. getAccountGroup
	res, err := doSdoAppRequest("getAccountGroup", map[string]string{"serviceUrl": "http://www.sdo.com", "tgt": tgt}, cfg, tgt)
	if err != nil {
		log.Printf("getAccountGroup error: %v\n", err)
		return
	}

	var sndaId string
	if data, ok := res["data"].(map[string]interface{}); ok {
		if sndaIds, ok := data["sndaIdArray"].([]interface{}); ok && len(sndaIds) > 0 {
			sndaId = fmt.Sprintf("%v", sndaIds[0])
		}
	}

	if sndaId == "" {
		log.Printf("无法获取 sndaId，升级失败: %v\n", res)
		return
	}

	// 2. accountGroupLogin
	res2, err := doSdoAppRequest("accountGroupLogin", map[string]string{
		"serviceUrl": "http://www.sdo.com",
		"tgt": tgt,
		"sndaId": sndaId,
		"autoLoginFlag": "1",
		"autoLoginKeepTime": "30",
	}, cfg, tgt)

	if err != nil {
		log.Printf("accountGroupLogin error: %v\n", err)
		return
	}

	if returnCode, ok := res2["return_code"].(float64); ok && returnCode == 0 {
		if data, ok := res2["data"].(map[string]interface{}); ok {
			if autoLoginKey, ok := data["autoLoginSessionKey"].(string); ok {
				cfg.AutoLoginKey = autoLoginKey
				log.Println("升级长效 App Session 成功！已保存 autoLoginSessionKey。")
				SaveConfig(configFile, cfg)
			}
		}
	} else {
		log.Printf("accountGroupLogin 失败: %v\n", res2)
	}
}

func refreshSdoAppSession(cfg *Config) bool {
	if cfg.AutoLoginKey == "" {
		return false
	}

	guid, err := getSdoGuid(cfg)
	if err != nil {
		log.Printf("获取 Guid 失败: %v\n", err)
		return false
	}

	log.Println("正在通过 App API 刷新长效 Session...")
	res, err := doSdoAppRequest("autoLogin.json", map[string]string{
		"autoLoginSessionKey": cfg.AutoLoginKey,
		"guid": guid,
	}, cfg, "")

	if err != nil {
		log.Printf("刷新长效 Session 失败: %v\n", err)
		return false
	}

	if returnCode, ok := res["return_code"].(float64); ok && returnCode == 0 {
		if data, ok := res["data"].(map[string]interface{}); ok {
			newKey, _ := data["autoLoginSessionKey"].(string)
			newTgt, _ := data["tgt"].(string)

			if newKey != "" && newTgt != "" {
				cfg.AutoLoginKey = newKey
				// 更新全局 Cookie 中的 CASTGC
				found := false
				for i, c := range cfg.Cookies {
					if c.Name == "CASTGC" {
						cfg.Cookies[i].Value = newTgt
						found = true
						break
					}
				}
				if !found {
					cfg.Cookies = append(cfg.Cookies, Cookie{
						Name: "CASTGC",
						Value: newTgt,
						Domain: ".sdo.com",
						Path: "/",
					})
				}
				SaveConfig(configFile, cfg)
				log.Println("长效 Session 刷新成功，已更新 CASTGC！有效期重新延长 30 天。")
				return true
			}
		}
	} else {
		log.Printf("长效 Session 刷新被拒绝 (可能已过期): %v\n", res)
		// 如果失效，清除 key 以便重新获取
		cfg.AutoLoginKey = ""
		SaveConfig(configFile, cfg)
	}

	return false
}
