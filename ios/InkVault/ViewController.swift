import UIKit
import WebKit

/// 墨库 InkVault iOS 外壳：内置离线知识库，导出时通过系统分享保存到「文件」。
final class ViewController: UIViewController, WKScriptMessageHandler, WKNavigationDelegate {

    private var webView: WKWebView!

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = UIColor(red: 0.965, green: 0.953, blue: 0.925, alpha: 1)

        let config = WKWebViewConfiguration()
        config.allowsInlineMediaPlayback = true
        config.mediaTypesRequiringUserActionForPlayback = []
        config.setValue(true, forKey: "allowUniversalAccessFromFileURLs")
        config.preferences.setValue(true, forKey: "allowFileAccessFromFileURLs")
        config.userContentController.add(self, name: "inkvault")

        webView = WKWebView(frame: .zero, configuration: config)
        webView.navigationDelegate = self
        webView.scrollView.contentInsetAdjustmentBehavior = .never
        webView.scrollView.bounces = false
        webView.isOpaque = false
        webView.backgroundColor = view.backgroundColor
        webView.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(webView)
        NSLayoutConstraint.activate([
            webView.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            webView.trailingAnchor.constraint(equalTo: view.trailingAnchor),
            webView.topAnchor.constraint(equalTo: view.topAnchor),
            webView.bottomAnchor.constraint(equalTo: view.bottomAnchor)
        ])

        guard let www = Bundle.main.url(forResource: "www", withExtension: nil) else {
            showError("缺少内置资源")
            return
        }
        let index = www.appendingPathComponent("index.html")
        webView.loadFileURL(index, allowingReadAccessTo: www)
    }

    private func showError(_ message: String) {
        let label = UILabel()
        label.text = message
        label.textColor = .darkGray
        label.textAlignment = .center
        label.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(label)
        NSLayoutConstraint.activate([
            label.centerXAnchor.constraint(equalTo: view.centerXAnchor),
            label.centerYAnchor.constraint(equalTo: view.centerYAnchor)
        ])
    }

    // MARK: - JS bridge

    func userContentController(_ userContentController: WKUserContentController,
                               didReceive message: WKScriptMessage) {
        guard message.name == "inkvault",
              let body = message.body as? [String: Any],
              body["action"] as? String == "save",
              let b64 = body["b64"] as? String,
              let data = Data(base64Encoded: b64, options: .ignoreUnknownCharacters) else { return }

        let rawName = (body["name"] as? String) ?? "inkvault-export"
        let name = rawName.replacingOccurrences(of: "/", with: "-").replacingOccurrences(of: ":", with: "-")
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(name)
        do {
            try data.write(to: url, options: .atomic)
        } catch {
            return
        }
        let activity = UIActivityViewController(activityItems: [url], applicationActivities: nil)
        if let pop = activity.popoverPresentationController {
            pop.sourceView = view
            pop.sourceRect = CGRect(x: view.bounds.midX, y: view.bounds.midY, width: 0, height: 0)
        }
        present(activity, animated: true)
    }

    // MARK: - Navigation

    func webView(_ webView: WKWebView,
                 decidePolicyFor navigationAction: WKNavigationAction,
                 decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let url = navigationAction.request.url else {
            decisionHandler(.allow)
            return
        }
        if url.isFileURL {
            decisionHandler(.allow)
            return
        }
        if navigationAction.targetFrame == nil {
            UIApplication.shared.open(url)
            decisionHandler(.cancel)
            return
        }
        decisionHandler(.allow)
    }
}
