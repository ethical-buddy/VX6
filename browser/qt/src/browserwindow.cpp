#include "browserwindow.h"

#include "vx6backend.h"
#include "vx6schemehandler.h"

#include <QAction>
#include <QDockWidget>
#include <QLineEdit>
#include <QListWidget>
#include <QMessageBox>
#include <QPushButton>
#include <QSettings>
#include <QStyle>
#include <QTabWidget>
#include <QListWidgetItem>
#include <QTextEdit>
#include <QToolBar>
#include <QVBoxLayout>
#include <QWebEnginePage>
#include <QWebEngineProfile>
#include <QWebEngineSettings>
#include <QWebEngineUrlRequestInfo>
#include <QWebEngineUrlRequestInterceptor>
#include <QWebEngineView>
#include <QUrl>

namespace {
class VX6RequestInterceptor : public QWebEngineUrlRequestInterceptor
{
public:
    explicit VX6RequestInterceptor(QObject *parent = nullptr)
        : QWebEngineUrlRequestInterceptor(parent)
    {
    }

    void interceptRequest(QWebEngineUrlRequestInfo &info) override
    {
        const QString scheme = info.requestUrl().scheme().toLower();
        if (scheme == "file" || scheme == "ftp") {
            info.block(true);
            return;
        }
        if (scheme == "javascript") {
            info.block(true);
        }
    }
};
} // namespace

BrowserWindow::BrowserWindow(const QString &vx6Binary, const QString &configPath, QWidget *parent)
    : QMainWindow(parent)
    , m_backend(new VX6Backend(vx6Binary, configPath, this))
    , m_profile(new QWebEngineProfile("vx6-browser", this))
    , m_tabs(nullptr)
    , m_address(nullptr)
    , m_logView(nullptr)
    , m_logDock(nullptr)
    , m_shortcuts(nullptr)
{
    setWindowTitle("VX6 Browser");
    resize(1480, 980);

    m_profile->setHttpUserAgent("VX6 Browser");
    m_profile->setUrlRequestInterceptor(new VX6RequestInterceptor(m_profile));
    m_profile->installUrlSchemeHandler(QByteArrayLiteral("vx6"), new VX6SchemeHandler(m_backend, m_profile));

    buildUi();
    buildToolbar();
    buildDock();
    registerBrowserCallbacks();
    maybeShowPermissionPrompt();
    openHome();
}

void BrowserWindow::buildUi()
{
    m_tabs = new QTabWidget(this);
    m_tabs->setDocumentMode(true);
    m_tabs->setTabsClosable(true);
    m_tabs->setMovable(true);
    connect(m_tabs, &QTabWidget::currentChanged, this, &BrowserWindow::currentTabChanged);
    connect(m_tabs, &QTabWidget::tabCloseRequested, this, &BrowserWindow::closeTab);
    setCentralWidget(m_tabs);
}

void BrowserWindow::buildToolbar()
{
    auto *toolbar = new QToolBar("Navigation", this);
    toolbar->setMovable(false);
    toolbar->setIconSize(QSize(18, 18));
    toolbar->setStyleSheet(
        "QToolBar { padding: 8px; spacing: 8px; background: #0b1324; border-bottom: 1px solid rgba(255,255,255,0.05); }"
        "QToolButton, QPushButton { background: #17233f; color: #eef4ff; border: 1px solid rgba(255,255,255,0.08); padding: 8px 12px; border-radius: 10px; }"
        "QToolButton:hover, QPushButton:hover { background: #223354; }"
        "QLineEdit { background: #101b31; color: #eef4ff; border: 1px solid rgba(255,255,255,0.08); border-radius: 12px; padding: 9px 12px; min-width: 520px; }");
    addToolBar(toolbar);

    QAction *back = toolbar->addAction(style()->standardIcon(QStyle::SP_ArrowBack), "Back");
    QAction *forward = toolbar->addAction(style()->standardIcon(QStyle::SP_ArrowForward), "Forward");
    QAction *reload = toolbar->addAction(style()->standardIcon(QStyle::SP_BrowserReload), "Reload");
    QAction *home = toolbar->addAction(style()->standardIcon(QStyle::SP_DirHomeIcon), "Home");
    QAction *bookmark = toolbar->addAction("Bookmark");
    QAction *newtab = toolbar->addAction("New Tab");
    QAction *logs = toolbar->addAction("Logs");

    m_address = new QLineEdit(this);
    m_address->setPlaceholderText("Enter vx6://, http://, https://, localhost, or a service/node target");
    toolbar->addWidget(m_address);

    connect(back, &QAction::triggered, this, [this] {
        if (auto *view = currentView()) {
            view->back();
        }
    });
    connect(forward, &QAction::triggered, this, [this] {
        if (auto *view = currentView()) {
            view->forward();
        }
    });
    connect(reload, &QAction::triggered, this, [this] {
        if (auto *view = currentView()) {
            view->reload();
        }
        refreshStatus();
    });
    connect(home, &QAction::triggered, this, &BrowserWindow::openHome);
    connect(bookmark, &QAction::triggered, this, &BrowserWindow::bookmarkCurrent);
    connect(newtab, &QAction::triggered, this, &BrowserWindow::newTab);
    connect(logs, &QAction::triggered, this, &BrowserWindow::toggleLogs);
    connect(m_address, &QLineEdit::returnPressed, this, &BrowserWindow::openAddress);
}

void BrowserWindow::buildDock()
{
    m_logDock = new QDockWidget("Node Logs", this);
    m_logDock->setAllowedAreas(Qt::LeftDockWidgetArea | Qt::RightDockWidgetArea);
    m_logDock->setFeatures(QDockWidget::DockWidgetClosable | QDockWidget::DockWidgetMovable);

    QWidget *dockBody = new QWidget(m_logDock);
    auto *layout = new QVBoxLayout(dockBody);
    layout->setContentsMargins(14, 14, 14, 14);
    layout->setSpacing(10);

    auto *reloadBtn = new QPushButton("Reload Node", dockBody);
    auto *statusBtn = new QPushButton("Refresh Status", dockBody);
    auto *permBtn = new QPushButton("Firewall / Admin Notes", dockBody);
    m_shortcuts = new QListWidget(dockBody);
    m_shortcuts->addItems({
        "vx6://status",
        "vx6://dht",
        "vx6://registry",
        "vx6://services",
        "vx6://peers",
        "vx6://identity",
        "vx6://permissions",
    });
    m_shortcuts->setStyleSheet("QListWidget { background: #101b31; color: #eef4ff; border: 1px solid rgba(255,255,255,0.08); border-radius: 12px; }");
    m_logView = new QTextEdit(dockBody);
    m_logView->setReadOnly(true);
    m_logView->setPlaceholderText("VX6 runtime and browser activity appear here.");
    m_logView->setStyleSheet("QTextEdit { background: #0c1426; color: #dfe7f9; border: 1px solid rgba(255,255,255,0.08); border-radius: 12px; }");

    layout->addWidget(reloadBtn);
    layout->addWidget(statusBtn);
    layout->addWidget(permBtn);
    layout->addWidget(m_shortcuts, 1);
    layout->addWidget(m_logView, 2);
    dockBody->setLayout(layout);
    m_logDock->setWidget(dockBody);
    addDockWidget(Qt::RightDockWidgetArea, m_logDock);

    connect(reloadBtn, &QPushButton::clicked, this, &BrowserWindow::reloadNode);
    connect(statusBtn, &QPushButton::clicked, this, &BrowserWindow::refreshStatus);
    connect(permBtn, &QPushButton::clicked, this, [this] {
        navigateTo("vx6://permissions", false);
    });
    connect(m_shortcuts, &QListWidget::itemDoubleClicked, this, [this](QListWidgetItem *item) {
        navigateTo(item->text(), false);
    });
}

void BrowserWindow::registerBrowserCallbacks()
{
    connect(m_backend, &VX6Backend::logLine, this, &BrowserWindow::appendLog);
}

void BrowserWindow::maybeShowPermissionPrompt()
{
    QSettings settings;
    if (settings.value("browser/permissions_acknowledged", false).toBool()) {
        return;
    }
    if (!m_backend->needsPermissionPrompt()) {
        settings.setValue("browser/permissions_acknowledged", true);
        return;
    }

    const auto result = QMessageBox::warning(
        this,
        "VX6 startup permissions",
        "VX6 Browser needs first-run firewall/admin guidance on this OS so the node backend can publish and connect cleanly later.\n\n"
        "Continue now and open the permissions page?",
        QMessageBox::Yes | QMessageBox::No,
        QMessageBox::Yes);
    if (result == QMessageBox::Yes) {
        settings.setValue("browser/permissions_acknowledged", true);
        navigateTo("vx6://permissions", false);
    }
}

QWebEngineView *BrowserWindow::createTab(const QUrl &initialUrl)
{
    auto *view = new QWebEngineView(this);
    auto *page = new QWebEnginePage(m_profile, view);
    view->setPage(page);
    view->settings()->setAttribute(QWebEngineSettings::JavascriptEnabled, true);
    view->settings()->setAttribute(QWebEngineSettings::LocalContentCanAccessRemoteUrls, false);

    connect(view, &QWebEngineView::urlChanged, this, [this, view](const QUrl &url) {
        if (view == currentView()) {
            m_address->setText(url.toString());
        }
    });
    connect(view, &QWebEngineView::titleChanged, this, [this, view](const QString &title) {
        const int idx = m_tabs->indexOf(view);
        if (idx >= 0) {
            m_tabs->setTabText(idx, title.isEmpty() ? "VX6" : title);
        }
    });
    connect(view, &QWebEngineView::loadFinished, this, [this, view](bool ok) {
        appendLog(QString("page %1: %2").arg(ok ? "loaded" : "failed", view->url().toString()));
    });

    view->setUrl(initialUrl);
    return view;
}

QWebEngineView *BrowserWindow::currentView() const
{
    return qobject_cast<QWebEngineView *>(m_tabs->currentWidget());
}

void BrowserWindow::openHome()
{
    if (m_tabs->count() == 0) {
        newTab();
    }
    navigateTo("vx6://home", false);
}

void BrowserWindow::newTab()
{
    const QUrl startUrl("vx6://home");
    auto *view = createTab(startUrl);
    const int idx = m_tabs->addTab(view, "VX6");
    m_tabs->setCurrentIndex(idx);
}

void BrowserWindow::closeTab(int index)
{
    if (m_tabs->count() <= 1) {
        return;
    }
    QWidget *tab = m_tabs->widget(index);
    m_tabs->removeTab(index);
    tab->deleteLater();
}

void BrowserWindow::currentTabChanged(int index)
{
    if (index < 0) {
        return;
    }
    if (auto *view = currentView()) {
        m_address->setText(view->url().toString());
    }
}

void BrowserWindow::toggleLogs()
{
    m_logDock->setVisible(!m_logDock->isVisible());
}

void BrowserWindow::reloadNode()
{
    appendLog("reloading vx6 runtime");
    const QString output = m_backend->runVX6(QStringList{"reload"});
    appendLog(output.trimmed());
    if (auto *view = currentView()) {
        view->reload();
    }
}

void BrowserWindow::refreshStatus()
{
    appendLog("refreshing status");
    const QString output = m_backend->runVX6(QStringList{"status"});
    appendLog(output.trimmed());
    if (currentView()) {
        currentView()->setUrl(QUrl("vx6://status"));
    }
}

void BrowserWindow::bookmarkCurrent()
{
    if (auto *view = currentView()) {
        const QString url = view->url().toString();
        if (!url.isEmpty()) {
            QSettings settings;
            QStringList bookmarks = settings.value("browser/bookmarks").toStringList();
            if (!bookmarks.contains(url)) {
                bookmarks.append(url);
                settings.setValue("browser/bookmarks", bookmarks);
                appendLog(QString("bookmarked %1").arg(url));
            }
        }
    }
}

QString BrowserWindow::normalizeTarget(const QString &raw) const
{
    QString target = raw.trimmed();
    if (target.isEmpty()) {
        return QStringLiteral("vx6://home");
    }

    if (target.startsWith("vx6://", Qt::CaseInsensitive) || target.startsWith("http://", Qt::CaseInsensitive) || target.startsWith("https://", Qt::CaseInsensitive)) {
        return target;
    }

    const QString lower = target.toLower();
    if (lower == "status" || lower == "dht" || lower == "registry" || lower == "services" || lower == "peers" || lower == "identity" || lower == "permissions" || lower.startsWith("service/") || lower.startsWith("node/") || lower.startsWith("node-id/") || lower.startsWith("key/")) {
        return QStringLiteral("vx6://%1").arg(target);
    }

    if (target.startsWith("localhost", Qt::CaseInsensitive) || target.startsWith("127.") || target.startsWith("[::1]") || target.contains(':')) {
        return QStringLiteral("http://%1").arg(target);
    }

    if (target.contains('.') && !target.contains(' ')) {
        return QStringLiteral("https://%1").arg(target);
    }

    return QStringLiteral("vx6://service/%1").arg(target);
}

void BrowserWindow::navigateTo(const QString &text, bool newTab)
{
    const QUrl url(normalizeTarget(text));
    if (newTab || !currentView()) {
        auto *view = createTab(url);
        const int idx = m_tabs->addTab(view, url.scheme() == "vx6" ? "VX6" : url.host());
        m_tabs->setCurrentIndex(idx);
    } else {
        currentView()->setUrl(url);
    }
    m_address->setText(url.toString());
    appendLog(QString("navigate %1").arg(url.toString()));
}

void BrowserWindow::openAddress()
{
    navigateTo(m_address->text(), false);
}

void BrowserWindow::appendLog(const QString &line)
{
    if (!m_logView) {
        return;
    }
    m_logView->append(line.trimmed());
}
