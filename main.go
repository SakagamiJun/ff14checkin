package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

const configFile = "config.json"

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// cookiesToStr 把 Cookie 结构体数组转成用于 HTTP Header 的字符串
func cookiesToStr(cookies []Cookie) string {
	var strs []string
	for _, c := range cookies {
		strs = append(strs, c.Name+"="+c.Value)
	}
	return strings.Join(strs, "; ")
}

// refreshCookies 启动一个无头浏览器访问目标 URL，过验后抓取完整 Cookie
func refreshCookies(targetURL string, currentCookies []Cookie) ([]Cookie, error) {
	fmt.Printf("正在通过 chromedp 刷新 Cookie [%s]...\n", targetURL)
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var newCookies []Cookie

	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			if len(currentCookies) == 0 {
				return nil
			}
			for _, c := range currentCookies {
				domain := c.Domain
				path := c.Path
				if path == "" {
					path = "/"
				}
				// 严格按照原有的 Domain 和 Path 注入
				network.SetCookie(c.Name, c.Value).WithDomain(domain).WithPath(path).Do(ctx)
			}
			return nil
		}),
		chromedp.Navigate(targetURL),
		chromedp.Sleep(5*time.Second),
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := storage.GetCookies().Do(ctx)
			if err != nil {
				return err
			}
			for _, c := range cookies {
				newCookies = append(newCookies, Cookie{
					Name:   c.Name,
					Value:  c.Value,
					Domain: c.Domain,
					Path:   c.Path,
				})
			}
			return nil
		}),
	)

	return newCookies, err
}

func interactiveLogin(targetURL string) ([]Cookie, error) {
	fmt.Printf("启动可视化浏览器，请在窗口中登录 [%s]...\n", targetURL)
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", true),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	var newCookies []Cookie

	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.ClearBrowserCookies().Do(ctx)
		}),
		chromedp.Navigate(targetURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Println("\n【注意】请在弹出的浏览器窗口中手动完成登录。")
			fmt.Println(">>> 登录成功后，请在终端按【回车键 (Enter)】继续抓取 Cookie... <<<")
			var dummy string
			fmt.Scanln(&dummy)
			fmt.Println("正在提取最新 Cookie...")
			cookies, err := storage.GetCookies().Do(ctx)
			if err != nil {
				return err
			}
			for _, c := range cookies {
				newCookies = append(newCookies, Cookie{
					Name:   c.Name,
					Value:  c.Value,
					Domain: c.Domain,
					Path:   c.Path,
				})
			}
			return nil
		}),
	)

	return newCookies, err
}

// mergeCookies 合并旧的 Cookie 结构数组和新的 Cookie 对象，返回合并后的数组和是否发生变化
func mergeCookies(oldCookies []Cookie, newHttpCookies []*http.Cookie) ([]Cookie, bool) {
	if len(newHttpCookies) == 0 {
		return oldCookies, false
	}

	cookieMap := make(map[string]Cookie)
	for _, c := range oldCookies {
		cookieMap[c.Name] = c
	}

	changed := false
	for _, hc := range newHttpCookies {
		if existing, ok := cookieMap[hc.Name]; !ok || existing.Value != hc.Value {
			domain := hc.Domain
			if domain == "" {
				domain = ".sdo.com"
			}
			path := hc.Path
			if path == "" {
				path = "/"
			}
			cookieMap[hc.Name] = Cookie{
				Name:   hc.Name,
				Value:  hc.Value,
				Domain: domain,
				Path:   path,
			}
			changed = true
		}
	}

	if !changed {
		return oldCookies, false
	}

	var out []Cookie
	var keys []string
	for k := range cookieMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, cookieMap[k])
	}
	return out, true
}

// preflightRequest 在调用正式 API 前，先访问主站以触发服务器下发最新的 Session Cookie
func preflightRequest(targetURL string, cfg *Config, task *TaskConfig) {
	fmt.Printf("执行签到前置请求以刷新 Session: %s\n", targetURL)
	req, _ := http.NewRequest("GET", targetURL, nil)
	req.Header.Set("Cookie", cookiesToStr(task.Cookies))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")

	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(15),
		tls_client.WithClientProfile(profiles.Chrome_124),
	}
	client, _ := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("前置请求异常: %v\n", err)
		return
	}
	defer resp.Body.Close()

	newCookies, changed := mergeCookies(task.Cookies, resp.Cookies())
	if changed {
		task.Cookies = newCookies
		SaveConfig(configFile, cfg)
		fmt.Println("前置请求成功，已获取最新 Session Cookie 并保存。")
	} else {
		fmt.Println("前置请求完成，未发现新的 Cookie。")
	}
}

// executeTask 执行单个签到任务
func executeTask(cfg *Config, taskName string) {
	task := cfg.GetTask(taskName)
	if task == nil {
		log.Printf("错误: 找不到任务 %s", taskName)
		return
	}

	fmt.Printf("\n--- 开始执行任务: %s ---\n", task.Name)

	// 1. 尝试无头模式刷新 Cookie
	newCookies, _ := refreshCookies(task.URL, task.Cookies)
	if len(newCookies) > 0 {
		task.Cookies = newCookies
		SaveConfig(configFile, cfg)
	}

	// 2. 发起 API 请求
	var needsLogin bool
	if task.Name == "ff14risingstones" {
		preflightRequest("https://ff14risingstones.web.sdo.com/", cfg, task)
		needsLogin = doFF14SignIn(cfg, task)
	} else if task.Name == "qu_sdo" {
		preflightRequest("https://qu.sdo.com/personal-center", cfg, task)
		needsLogin = doQuSignIn(cfg, task)
	}

	// 3. 手动接管逻辑
	if needsLogin {
		fmt.Printf("任务 %s 身份失效或需要验证，进入手动接管模式...\n", task.Name)
		freshCookies, err := interactiveLogin(task.URL)
		if err == nil && len(freshCookies) > 0 {
			task.Cookies = freshCookies
			SaveConfig(configFile, cfg)
			fmt.Println("身份已更新，进行最终尝试...")
			if task.Name == "ff14risingstones" {
				doFF14SignIn(cfg, task)
			} else if task.Name == "qu_sdo" {
				doQuSignIn(cfg, task)
			}
		}
	}
}

func doFF14SignIn(cfg *Config, task *TaskConfig) bool {
	apiURL := "https://apiff14risingstones.web.sdo.com/api/home/sign/signIn"
	uuid := generateUUID()
	u, _ := url.Parse(apiURL)
	q := u.Query()
	q.Set("tempsuid", uuid)
	u.RawQuery = q.Encode()

	formData := url.Values{}
	formData.Set("tempsuid", uuid)
	
	req, _ := http.NewRequest("POST", u.String(), strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookiesToStr(task.Cookies))
	req.Header.Set("Referer", "https://ff14risingstones.web.sdo.com/")
	req.Header.Set("Origin", "https://ff14risingstones.web.sdo.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")

	needsLogin, newCookies := performRequest(req, task.Cookies)
	if len(newCookies) > 0 {
		task.Cookies = newCookies
		SaveConfig(configFile, cfg)
		fmt.Println("API 响应中检测到 Cookie 更新，已自动保存。")
	}
	return needsLogin
}

func doQuSignIn(cfg *Config, task *TaskConfig) bool {
	// qu.sdo.com 的签到接口是 PUT，且有特殊的 Header
	apiURL := "https://sqmallservice.u.sdo.com/api/us/integration/checkIn?merchantId=1"
	
	req, _ := http.NewRequest("PUT", apiURL, nil)
	req.Header.Set("qu-merchant-id", "1")
	req.Header.Set("qu-hardware-platform", "3")
	req.Header.Set("qu-software-platform", "1")
	req.Header.Set("qu-deploy-platform", "1")
	req.Header.Set("qu-web-host", "qu.sdo.com")
	req.Header.Set("Cookie", cookiesToStr(task.Cookies))
	req.Header.Set("Referer", "https://qu.sdo.com/")
	req.Header.Set("Origin", "https://qu.sdo.com")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")

	needsLogin, newCookies := performRequest(req, task.Cookies)
	if len(newCookies) > 0 {
		task.Cookies = newCookies
		SaveConfig(configFile, cfg)
		fmt.Println("API 响应中检测到 Cookie 更新，已自动保存。")
	}
	return needsLogin
}

func performRequest(req *http.Request, currentCookies []Cookie) (bool, []Cookie) {
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(15),
		tls_client.WithClientProfile(profiles.Chrome_124),
	}
	client, _ := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("请求异常: %v\n", err)
		return true, nil
	}
	defer resp.Body.Close()

	newCookies, changed := mergeCookies(currentCookies, resp.Cookies())
	if !changed {
		newCookies = nil
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	fmt.Printf("API HTTP 状态码: %d\n", resp.StatusCode)

	needsLogin := false
	if resp.StatusCode != 200 {
		needsLogin = true
	}

	var jsonObj map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &jsonObj); err == nil {
		pretty, _ := json.MarshalIndent(jsonObj, "", "  ")
		fmt.Printf("响应内容: \n%s\n", string(pretty))
		if code, ok := jsonObj["code"].(float64); ok && (code == 10105 || code == 10403 || code == -10350174) {
			needsLogin = true
		}
		// 趣商城的返回码是 resultCode
		if code, ok := jsonObj["resultCode"].(float64); ok && (code == -10350174) {
			needsLogin = true
		}
	} else {
		needsLogin = true
	}
	return needsLogin, newCookies
}

func main() {
	// 网络检查逻辑
	if !waitForNetwork() {
		slog.Error("网络检查失败，重试次数耗尽，程序退出")
		os.Exit(1)
	}

	cfg, err := LoadConfig(configFile)
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	// 初始化默认任务（如果配置中没有任务）
	if len(cfg.Tasks) == 0 {
		cfg.Tasks = append(cfg.Tasks, TaskConfig{Name: "ff14risingstones", URL: "https://ff14risingstones.web.sdo.com/pc/index.html#/post"})
		cfg.Tasks = append(cfg.Tasks, TaskConfig{Name: "qu_sdo", URL: "https://qu.sdo.com/personal-center?merchantId=1#pointsindex-1"})
		SaveConfig(configFile, cfg)
	}

	for i := range cfg.Tasks {
		// executeTask 可能会修改 cfg，传指针
		executeTask(cfg, cfg.Tasks[i].Name)
	}
}

// waitForNetwork 检查网络连接，失败则重试
func waitForNetwork() bool {
	for i := 1; i <= 5; i++ {
		// 尝试连接盛大首页，超时时间 5s
		conn, err := net.DialTimeout("tcp", "www.sdo.com:443", 5*time.Second)
		if err == nil {
			conn.Close()
			slog.Info("网络检查通过")
			return true
		}

		slog.Warn("网络连接异常，将在 120s 后重试", "attempt", i, "max_attempts", 5, "error", err)
		if i < 5 {
			time.Sleep(120 * time.Second)
		}
	}
	return false
}