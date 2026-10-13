package spider

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/LouYuanbo1/ai-spider/config"
	"github.com/go-rod/rod"
	"github.com/go-rod/stealth"
)

type Spider struct {
	browser  *rod.Browser
	page     *rod.Page
	logger   *slog.Logger
	humanize bool
}

func InitSpider(cfg *config.RodConfig) (*Spider, error) {
	logger := slog.Default().With("component", "spider")

	// Chrome 136+ 要求远程调试必须搭配非默认的 --user-data-dir，
	// 无论是否 user 模式都必须显式设置，否则启动会被拒绝。
	// 路径不得为""
	if cfg.UserDataDir != "" {
		if err := os.MkdirAll(cfg.UserDataDir, 0755); err != nil {
			return nil, fmt.Errorf("创建实例数据目录失败: %v", err)
		}
	}

	launcherOptions := []LauncherOption{
		WithBin(cfg.Bin),
		WithHeadless(cfg.Headless),
		WithDisableBlinkFeatures(cfg.DisableBlinkFeatures),
		WithIncognito(cfg.Incognito),
		WithDisableDevShmUsage(cfg.DisableDevShmUsage),
		WithNoSandbox(cfg.NoSandbox),
		WithUserAgent(cfg.UserAgent),
		WithLeakless(cfg.Leakless),
		WithDisableBackgroundNetworking(cfg.DisableBackgroundNetworking),
		WithDisableBackgroundTimerThrottling(cfg.DisableBackgroundTimerThrottling),
		WithDisableBackgroundingOccludedWindows(cfg.DisableBackgroundingOccludedWindows),
		WithDisableRendererBackgrounding(cfg.DisableRendererBackgrounding),
		WithRemoteDebuggingPort(cfg.BasicRemoteDebuggingPort),
		WithWindowSize(cfg.DefaultPageWidth, cfg.DefaultPageHeight),
		WithUserDataDir(cfg.UserDataDir),
	}

	launcher := CreateLauncher(cfg.UserMode, launcherOptions...)
	url, err := launcher.Launch()
	if err != nil {
		return nil, fmt.Errorf("启动浏览器失败: %v", err)
	}
	logger.Info("浏览器就绪", "instance", 1, "url", url)

	browser := rod.New().ControlURL(url).Trace(cfg.Trace)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("连接浏览器失败: %v", err)
	}

	page := stealth.MustPage(browser)

	return &Spider{
		browser:  browser,
		page:     page,
		logger:   logger,
		humanize: cfg.Humanize,
	}, nil
}

// Close 关闭爬虫，释放所有浏览器资源
func (s *Spider) Close() error {
	s.logger.Info("关闭爬虫")
	if err := s.browser.Close(); err != nil {
		return fmt.Errorf("关闭浏览器失败: %v", err)
	}
	return nil
}

func (s *Spider) ClosePage() error {
	if err := s.page.Close(); err != nil {
		return fmt.Errorf("关闭页面失败: %v", err)
	}
	return nil
}

// Navigate 导航到目标URL并等待页面加载与网络空闲
func (s *Spider) Navigate(url string) error {
	if err := s.page.Navigate(url); err != nil {
		return fmt.Errorf("导航失败: %v", err)
	}
	if err := s.page.WaitLoad(); err != nil {
		if strings.Contains(err.Error(), "navigated or closed") {
			return nil
		}
		return fmt.Errorf("页面加载等待失败: %v", err)
	}
	s.page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)
	return nil
}
