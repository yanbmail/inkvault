package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"inkvault/internal/api"
	"inkvault/internal/store"
)

//go:embed all:web
var embeddedWeb embed.FS

const version = "1.0.0"

func main() {
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
	mime.AddExtensionType(".md", "text/markdown; charset=utf-8")

	var (
		port      = flag.Int("port", 0, "服务端口（默认 8790）")
		dataDir   = flag.String("data", "", "数据目录（默认程序同级的 data 文件夹）")
		noBrowser = flag.Bool("no-browser", false, "启动后不自动打开浏览器")
		bindHost  = flag.String("bind", "0.0.0.0", "监听地址，0.0.0.0 表示同时允许局域网/花生壳访问")
		showVer   = flag.Bool("version", false, "显示版本号")
	)
	flag.Usage = usage
	flag.Parse()
	if *showVer {
		fmt.Printf("InkVault 墨库 %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		return
	}

	dir := *dataDir
	if dir == "" {
		dir = defaultDataDir()
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	cfg, err := api.LoadConfig(dir, 8790)
	if err != nil {
		fatal("读取配置失败: %v", err)
	}
	if *port > 0 {
		cfg.Port = *port
		_ = api.SaveConfig(dir, cfg)
	}

	db, err := store.Open(dir)
	if err != nil {
		fatal("打开数据库失败: %v", err)
	}
	defer db.Close()

	webFS, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		fatal("加载前端资源失败: %v", err)
	}
	srv := api.New(db, cfg, dir, webFS)

	addr := net.JoinHostPort(*bindHost, strconv.Itoa(cfg.Port))
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	printBanner(cfg, dir, db)

	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	if !*noBrowser {
		time.Sleep(350 * time.Millisecond)
		openBrowser(fmt.Sprintf("http://127.0.0.1:%d/?pin=%s", cfg.Port, cfg.PIN))
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		fatal("服务启动失败: %v", err)
	case <-stop:
		fmt.Println("\n正在退出，数据已安全写入磁盘……")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `墨库 InkVault %s —— 本地优先的个人笔记知识库

用法:
  inkvault [选项]

选项:
  -port 8790       服务端口
  -data <目录>      指定数据目录（默认程序同级 data/）
  -bind 0.0.0.0    监听地址
  -no-browser      启动后不打开浏览器
  -version         显示版本号

数据默认保存在:
  %s
`, version, defaultDataDir())
	flag.PrintDefaults()
}

func defaultDataDir() string {
	exe, err := os.Executable()
	if err == nil {
		base := filepath.Dir(exe)
		// inside a .app bundle: InkVault.app/Contents/MacOS/inkvault
		if strings.Contains(base, ".app/Contents/MacOS") {
			return filepath.Join(homeDir(), "InkVault")
		}
		return filepath.Join(base, "data")
	}
	return filepath.Join(homeDir(), "InkVault")
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

func printBanner(cfg *api.Config, dir string, db *store.DB) {
	fmt.Println()
	fmt.Println("  ██ 墨库 InkVault " + version + "  ·  本地优先的个人知识库")
	fmt.Println("  ─────────────────────────────────────────────")
	fmt.Printf("  数据目录   %s\n", dir)
	fmt.Printf("  数据库     %s (%.1f MB)\n", db.DBPath, fileSizeMB(db.DBPath))
	fmt.Printf("  访问码     %s\n", cfg.PIN)
	fmt.Printf("  本机访问   http://127.0.0.1:%d\n", cfg.Port)
	for _, u := range lanURLs(cfg.Port) {
		fmt.Printf("  局域网     %s\n", u)
	}
	if cfg.PublicURL != "" {
		fmt.Printf("  公网地址   %s\n", cfg.PublicURL)
	}
	fmt.Println("  ─────────────────────────────────────────────")
	fmt.Println("  手机/外网访问：在同一网络或花生壳映射后，打开上面的地址并输入访问码。")
	fmt.Println("  按 Ctrl+C 退出。数据完全保存在本机，记得定期在「设置 → 备份」中导出。")
	fmt.Println()
}

func lanURLs(port int) []string {
	out := []string{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				if ip := ipnet.IP.To4(); ip != nil && !ip.IsLoopback() {
					out = append(out, fmt.Sprintf("http://%s:%d", ip.String(), port))
				}
			}
		}
	}
	return out
}

func fileSizeMB(path string) float64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return float64(fi.Size()) / 1024 / 1024
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		err = exec.Command("xdg-open", url).Start()
	}
	if err != nil {
		log.Printf("请手动打开：%s", url)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\n  ✗ "+format+"\n\n", args...)
	os.Exit(1)
}
