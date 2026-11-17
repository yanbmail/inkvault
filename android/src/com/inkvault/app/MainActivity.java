package com.inkvault.app;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.content.ContentValues;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Environment;
import android.util.Base64;
import android.util.Log;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.webkit.CookieManager;
import android.webkit.DownloadListener;
import android.webkit.JavascriptInterface;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLDecoder;

/**
 * 墨库 InkVault Android 外壳。
 * 默认加载内置的离线知识库（assets/www），也可以连接电脑或花生壳上的服务端。
 */
public class MainActivity extends Activity {

    private WebView webView;
    private LinearLayout setupPanel;
    private EditText urlInput;
    private EditText pinInput;
    private SharedPreferences prefs;
    private ValueCallback<Uri[]> filePathCallback;
    private static final int FILE_CHOOSER = 1001;
    private long lastBack = 0L;

    private static final int BG = Color.parseColor("#f6f3ec");
    private static final int INK = Color.parseColor("#211f1c");
    private static final int MUTED = Color.parseColor("#7d7568");
    private static final int ACCENT = Color.parseColor("#a8442f");
    private static final int LINE = Color.parseColor("#e6e0d4");

    @SuppressLint("SetJavaScriptEnabled")
    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        prefs = getSharedPreferences("inkvault", Context.MODE_PRIVATE);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
            getWindow().setStatusBarColor(BG);
            getWindow().setNavigationBarColor(BG);
            getWindow().clearFlags(WindowManager.LayoutParams.FLAG_TRANSLUCENT_STATUS);
            getWindow().getDecorView().setSystemUiVisibility(
                    View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR | View.SYSTEM_UI_FLAG_LAYOUT_STABLE);
        }

        FrameLayout root = new FrameLayout(this);
        webView = new WebView(this);
        webView.setBackgroundColor(BG);

        WebSettings s = webView.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setDatabaseEnabled(true);
        s.setAllowFileAccess(true);
        s.setAllowContentAccess(true);
        try {
            s.setAllowFileAccessFromFileURLs(true);
            s.setAllowUniversalAccessFromFileURLs(true);
        } catch (Throwable ignored) { }
        s.setUseWideViewPort(true);
        s.setLoadWithOverviewMode(true);
        s.setSupportZoom(false);
        s.setBuiltInZoomControls(false);
        s.setDisplayZoomControls(false);
        s.setMediaPlaybackRequiresUserGesture(false);
        s.setCacheMode(WebSettings.LOAD_DEFAULT);
        s.setTextZoom(100);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
            s.setMixedContentMode(WebSettings.MIXED_CONTENT_ALWAYS_ALLOW);
        }
        s.setUserAgentString(s.getUserAgentString() + " InkVaultApp/1.0");
        CookieManager.getInstance().setAcceptCookie(true);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
            CookieManager.getInstance().setAcceptThirdPartyCookies(webView, true);
        }
        WebView.setWebContentsDebuggingEnabled(true);

        webView.addJavascriptInterface(new Bridge(), "AndroidBridge");

        webView.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                return handleExternal(url);
            }

            @Override
            public boolean shouldOverrideUrlLoading(WebView view, android.webkit.WebResourceRequest request) {
                return handleExternal(request.getUrl().toString());
            }

            @Override
            public void onReceivedError(WebView view, android.webkit.WebResourceRequest request, android.webkit.WebResourceError error) {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP && request != null && request.isForMainFrame()) {
                    Toast.makeText(MainActivity.this, "无法连接服务端，请检查地址与网络", Toast.LENGTH_LONG).show();
                    showSetup();
                }
            }
        });

        webView.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback, FileChooserParams params) {
                if (filePathCallback != null) {
                    filePathCallback.onReceiveValue(null);
                }
                filePathCallback = callback;
                try {
                    Intent intent = new Intent(Intent.ACTION_GET_CONTENT);
                    intent.addCategory(Intent.CATEGORY_OPENABLE);
                    intent.setType("*/*");
                    startActivityForResult(Intent.createChooser(intent, "选择笔记文件"), FILE_CHOOSER);
                    return true;
                } catch (Exception e) {
                    filePathCallback = null;
                    return false;
                }
            }

            @Override
            public boolean onConsoleMessage(android.webkit.ConsoleMessage msg) {
                Log.d("InkVaultWeb", msg.message() + " @" + msg.lineNumber());
                return true;
            }
        });

        webView.setDownloadListener(new DownloadListener() {
            @Override
            public void onDownloadStart(String url, String userAgent, String contentDisposition, String mimeType, long contentLength) {
                startDownload(url, guessName(contentDisposition, mimeType));
            }
        });

        setupPanel = buildSetupPanel();
        root.addView(webView, new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        root.addView(setupPanel, new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        setContentView(root);

        String mode = prefs.getString("mode", "");
        if ("server".equals(mode)) {
            loadServer(prefs.getString("server", ""), prefs.getString("pin", ""));
        } else if ("local".equals(mode)) {
            loadLocal();
        } else {
            showSetup();
        }
    }

    private boolean handleExternal(String url) {
        if (url.startsWith("tel:") || url.startsWith("mailto:") || url.startsWith("sms:")) {
            try {
                startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(url)));
            } catch (Exception ignored) { }
            return true;
        }
        if (url.startsWith("file:///android_asset/")) {
            return false;
        }
        if (url.startsWith("http://") || url.startsWith("https://")) {
            String server = prefs.getString("server", "");
            if (!server.isEmpty() && url.startsWith(server)) {
                return false;
            }
        }
        return false;
    }

    /* --------------------------------- UI ---------------------------------- */

    private LinearLayout buildSetupPanel() {
        LinearLayout panel = new LinearLayout(this);
        panel.setOrientation(LinearLayout.VERTICAL);
        panel.setBackgroundColor(BG);
        panel.setGravity(Gravity.CENTER_HORIZONTAL);
        panel.setPadding(dp(26), dp(40), dp(26), dp(26));

        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        LinearLayout inner = new LinearLayout(this);
        inner.setOrientation(LinearLayout.VERTICAL);
        inner.setGravity(Gravity.CENTER_HORIZONTAL);
        inner.setPadding(dp(6), dp(20), dp(6), dp(20));
        scroll.addView(inner, new ScrollView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        TextView seal = new TextView(this);
        seal.setText("墨");
        seal.setTextSize(34);
        seal.setTextColor(Color.WHITE);
        seal.setGravity(Gravity.CENTER);
        GradientDrawable sealBg = new GradientDrawable();
        sealBg.setColor(ACCENT);
        sealBg.setCornerRadius(dp(18));
        seal.setBackground(sealBg);
        LinearLayout.LayoutParams sealLp = new LinearLayout.LayoutParams(dp(64), dp(64));
        sealLp.bottomMargin = dp(18);
        inner.addView(seal, sealLp);

        TextView title = new TextView(this);
        title.setText("墨库 InkVault");
        title.setTextSize(25);
        title.setTextColor(INK);
        title.setGravity(Gravity.CENTER);
        inner.addView(title);

        TextView sub = new TextView(this);
        sub.setText("本地优先的个人知识库");
        sub.setTextSize(12);
        sub.setTextColor(MUTED);
        sub.setGravity(Gravity.CENTER);
        LinearLayout.LayoutParams subLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        subLp.bottomMargin = dp(30);
        inner.addView(sub, subLp);

        TextView hint = new TextView(this);
        hint.setText("手机可以离线写作，所有内容保存在本机；\n也可以连接电脑或花生壳映射的服务端，共用同一套数据。");
        hint.setTextSize(12);
        hint.setTextColor(MUTED);
        hint.setLineSpacing(0, 1.6f);
        hint.setGravity(Gravity.CENTER);
        LinearLayout.LayoutParams hintLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        hintLp.bottomMargin = dp(26);
        inner.addView(hint, hintLp);

        Button localBtn = new Button(this);
        localBtn.setText("直接开始（离线模式）");
        localBtn.setAllCaps(false);
        localBtn.setTextSize(15);
        localBtn.setTextColor(Color.WHITE);
        GradientDrawable primaryBg = new GradientDrawable();
        primaryBg.setColor(ACCENT);
        primaryBg.setCornerRadius(dp(12));
        localBtn.setBackground(primaryBg);
        LinearLayout.LayoutParams btnLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, dp(48));
        inner.addView(localBtn, btnLp);
        localBtn.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                prefs.edit().putString("mode", "local").apply();
                loadLocal();
            }
        });

        TextView divider = new TextView(this);
        divider.setText("或连接电脑上的墨库");
        divider.setTextSize(11);
        divider.setTextColor(MUTED);
        divider.setGravity(Gravity.CENTER);
        LinearLayout.LayoutParams divLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        divLp.topMargin = dp(26);
        divLp.bottomMargin = dp(10);
        inner.addView(divider, divLp);

        TextView urlLabel = new TextView(this);
        urlLabel.setText("服务器地址");
        urlLabel.setTextSize(11);
        urlLabel.setTextColor(MUTED);
        inner.addView(urlLabel);

        urlInput = new EditText(this);
        urlInput.setHint("192.168.1.10:8790 或花生壳域名");
        urlInput.setTextSize(14);
        urlInput.setSingleLine(true);
        urlInput.setText(prefs.getString("server", ""));
        LinearLayout.LayoutParams inputLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        inputLp.topMargin = dp(4);
        inputLp.bottomMargin = dp(12);
        inner.addView(urlInput, inputLp);

        TextView pinLabel = new TextView(this);
        pinLabel.setText("访问码");
        pinLabel.setTextSize(11);
        pinLabel.setTextColor(MUTED);
        inner.addView(pinLabel);

        pinInput = new EditText(this);
        pinInput.setHint("启动窗口显示的 6 位访问码");
        pinInput.setTextSize(14);
        pinInput.setSingleLine(true);
        pinInput.setText(prefs.getString("pin", ""));
        LinearLayout.LayoutParams pinLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        pinLp.topMargin = dp(4);
        pinLp.bottomMargin = dp(16);
        inner.addView(pinInput, pinLp);

        Button connect = new Button(this);
        connect.setText("连接");
        connect.setAllCaps(false);
        connect.setTextSize(15);
        connect.setTextColor(INK);
        GradientDrawable outline = new GradientDrawable();
        outline.setColor(Color.TRANSPARENT);
        outline.setStroke(dp(1), LINE);
        outline.setCornerRadius(dp(12));
        connect.setBackground(outline);
        inner.addView(connect, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, dp(46)));
        connect.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                connectToServer();
            }
        });

        TextView tip = new TextView(this);
        tip.setText("提示：同一 Wi-Fi 下填电脑的局域网地址；外网使用花生壳映射后填分配到的域名。连接后会记住设置。");
        tip.setTextSize(10.5f);
        tip.setTextColor(MUTED);
        tip.setLineSpacing(0, 1.5f);
        LinearLayout.LayoutParams tipLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        tipLp.topMargin = dp(18);
        inner.addView(tip, tipLp);

        panel.addView(scroll, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        return panel;
    }

    private void showSetup() {
        if (setupPanel != null) {
            setupPanel.setVisibility(View.VISIBLE);
        }
        if (urlInput != null) {
            urlInput.setText(prefs.getString("server", ""));
        }
        if (pinInput != null) {
            pinInput.setText(prefs.getString("pin", ""));
        }
    }

    private void connectToServer() {
        String raw = urlInput.getText().toString().trim();
        if (raw.isEmpty()) {
            Toast.makeText(this, "请输入服务器地址", Toast.LENGTH_SHORT).show();
            return;
        }
        if (!raw.startsWith("http://") && !raw.startsWith("https://")) {
            raw = "http://" + raw;
        }
        if (!raw.matches(".*:\\d+$")) {
            raw = raw + ":8790";
        }
        String pin = pinInput.getText().toString().trim();
        prefs.edit().putString("mode", "server").putString("server", raw).putString("pin", pin).apply();
        loadServer(raw, pin);
    }

    private void loadLocal() {
        if (setupPanel != null) setupPanel.setVisibility(View.GONE);
        webView.loadUrl("file:///android_asset/www/index.html?local=1");
    }

    private void loadServer(String base, String pin) {
        if (setupPanel != null) setupPanel.setVisibility(View.GONE);
        if (base == null || base.isEmpty()) {
            showSetup();
            return;
        }
        String url = base;
        if (!pin.isEmpty()) {
            url = base + "/?pin=" + Uri.encode(pin);
        }
        webView.loadUrl(url);
    }

    private int dp(int v) {
        return (int) (v * getResources().getDisplayMetrics().density);
    }

    /* ------------------------------- downloads ------------------------------ */

    private String guessName(String contentDisposition, String mime) {
        if (contentDisposition != null) {
            java.util.regex.Matcher m = java.util.regex.Pattern
                    .compile("filename\\*?=(?:UTF-8'')?\"?([^\";]+)\"?").matcher(contentDisposition);
            if (m.find()) {
                try {
                    return URLDecoder.decode(m.group(1), "UTF-8");
                } catch (Exception e) {
                    return m.group(1);
                }
            }
        }
        String ext = ".bin";
        if (mime != null && mime.contains("markdown")) ext = ".md";
        else if (mime != null && mime.contains("zip")) ext = ".zip";
        else if (mime != null && mime.contains("json")) ext = ".json";
        return "inkvault-export" + ext;
    }

    private String mimeOf(String name) {
        String n = name.toLowerCase();
        if (n.endsWith(".xlsx")) return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";
        if (n.endsWith(".docx")) return "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
        if (n.endsWith(".csv")) return "text/csv";
        if (n.endsWith(".md")) return "text/markdown";
        if (n.endsWith(".html")) return "text/html";
        if (n.endsWith(".json")) return "application/json";
        if (n.endsWith(".zip")) return "application/zip";
        if (n.endsWith(".png")) return "image/png";
        return "application/octet-stream";
    }

    private void startDownload(final String url, final String name) {
        Toast.makeText(this, "开始下载：" + name, Toast.LENGTH_SHORT).show();
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    HttpURLConnection conn = (HttpURLConnection) new URL(url).openConnection();
                    conn.setConnectTimeout(15000);
                    conn.setReadTimeout(120000);
                    conn.setInstanceFollowRedirects(true);
                    conn.setRequestProperty("User-Agent", "InkVaultApp/1.0");
                    int code = conn.getResponseCode();
                    if (code < 200 || code >= 300) throw new java.io.IOException("服务器返回 " + code);
                    String saved = saveToDownloads(conn.getInputStream(), name, mimeOf(name));
                    final String msg = saved;
                    runOnUiThread(new Runnable() {
                        @Override
                        public void run() {
                            Toast.makeText(MainActivity.this, "已保存到 " + msg, Toast.LENGTH_LONG).show();
                        }
                    });
                } catch (final Exception e) {
                    runOnUiThread(new Runnable() {
                        @Override
                        public void run() {
                            Toast.makeText(MainActivity.this, "下载失败：" + e.getMessage(), Toast.LENGTH_LONG).show();
                        }
                    });
                }
            }
        }).start();
    }

    private String saveToDownloads(InputStream input, String name, String mime) throws Exception {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            ContentValues values = new ContentValues();
            values.put(android.provider.MediaStore.Downloads.DISPLAY_NAME, name);
            values.put(android.provider.MediaStore.Downloads.MIME_TYPE, mime);
            values.put(android.provider.MediaStore.Downloads.IS_PENDING, 1);
            Uri uri = getContentResolver().insert(android.provider.MediaStore.Downloads.EXTERNAL_CONTENT_URI, values);
            if (uri == null) throw new java.io.IOException("无法创建下载文件");
            OutputStream out = getContentResolver().openOutputStream(uri);
            if (out == null) throw new java.io.IOException("无法写入下载文件");
            copy(input, out);
            values.clear();
            values.put(android.provider.MediaStore.Downloads.IS_PENDING, 0);
            getContentResolver().update(uri, values, null, null);
            return "下载/" + name;
        }
        File dir = getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS);
        if (dir == null) dir = new File(getFilesDir(), "Download");
        if (!dir.exists()) dir.mkdirs();
        File file = new File(dir, name);
        FileOutputStream out = new FileOutputStream(file);
        copy(input, out);
        return file.getAbsolutePath();
    }

    private void copy(InputStream in, OutputStream out) throws Exception {
        try {
            byte[] buf = new byte[64 * 1024];
            int n;
            while ((n = in.read(buf)) > 0) {
                out.write(buf, 0, n);
            }
        } finally {
            try { in.close(); } catch (Exception ignored) { }
            try { out.close(); } catch (Exception ignored) { }
        }
    }

    /* --------------------------------- bridge ------------------------------ */

    public class Bridge {
        @JavascriptInterface
        public void saveBase64(String name, String mime, String base64) {
            try {
                byte[] data = Base64.decode(base64, Base64.DEFAULT);
                String safe = name == null || name.isEmpty() ? "inkvault-export" : name.replaceAll("[/\\\\:*?\"<>|]", "-");
                String saved = saveToDownloads(new java.io.ByteArrayInputStream(data), safe, mime == null ? "application/octet-stream" : mime);
                final String msg = saved;
                runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        Toast.makeText(MainActivity.this, "已保存到 " + msg, Toast.LENGTH_LONG).show();
                    }
                });
            } catch (final Exception e) {
                runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        Toast.makeText(MainActivity.this, "保存失败：" + e.getMessage(), Toast.LENGTH_LONG).show();
                    }
                });
            }
        }

        @JavascriptInterface
        public void download(String url, String name) {
            startDownload(url, name);
        }

        @JavascriptInterface
        public void openSetup() {
            runOnUiThread(new Runnable() {
                @Override
                public void run() {
                    showSetup();
                }
            });
        }

        @JavascriptInterface
        public void useLocal() {
            runOnUiThread(new Runnable() {
                @Override
                public void run() {
                    prefs.edit().putString("mode", "local").apply();
                    loadLocal();
                }
            });
        }

        @JavascriptInterface
        public String version() {
            return "1.0.0";
        }

        @JavascriptInterface
        public boolean isApp() {
            return true;
        }
    }

    /* -------------------------------- lifecycle ----------------------------- */

    @Override
    public void onActivityResult(int requestCode, int resultCode, Intent data) {
        if (requestCode == FILE_CHOOSER) {
            Uri[] results = null;
            if (resultCode == RESULT_OK && data != null) {
                if (data.getData() != null) {
                    results = new Uri[]{data.getData()};
                } else if (data.getClipData() != null) {
                    int count = data.getClipData().getItemCount();
                    results = new Uri[count];
                    for (int i = 0; i < count; i++) {
                        results[i] = data.getClipData().getItemAt(i).getUri();
                    }
                }
            }
            if (filePathCallback != null) {
                filePathCallback.onReceiveValue(results);
                filePathCallback = null;
            }
            return;
        }
        super.onActivityResult(requestCode, resultCode, data);
    }

    @Override
    public boolean onKeyDown(int keyCode, KeyEvent event) {
        if (keyCode == KeyEvent.KEYCODE_BACK) {
            if (setupPanel != null && setupPanel.getVisibility() == View.VISIBLE) {
                return super.onKeyDown(keyCode, event);
            }
            if (webView.canGoBack()) {
                webView.goBack();
                return true;
            }
            long now = System.currentTimeMillis();
            if (now - lastBack < 2000) return super.onKeyDown(keyCode, event);
            lastBack = now;
            Toast.makeText(this, "再按一次退出", Toast.LENGTH_SHORT).show();
            return true;
        }
        return super.onKeyDown(keyCode, event);
    }

    @Override
    protected void onResume() {
        super.onResume();
        webView.onResume();
    }

    @Override
    protected void onPause() {
        webView.onPause();
        super.onPause();
    }

    @Override
    protected void onDestroy() {
        if (webView != null) {
            webView.loadUrl("about:blank");
            webView.destroy();
        }
        super.onDestroy();
    }
}
