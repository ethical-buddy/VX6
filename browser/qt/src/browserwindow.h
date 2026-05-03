#pragma once

#include <QMainWindow>
#include <QString>
#include <QUrl>

class QDockWidget;
class QLineEdit;
class QListWidget;
class QTabWidget;
class QTextEdit;
class QWebEnginePage;
class QWebEngineProfile;
class QWebEngineView;
class VX6Backend;

class BrowserWindow : public QMainWindow
{
    Q_OBJECT

public:
    explicit BrowserWindow(const QString &vx6Binary, const QString &configPath, QWidget *parent = nullptr);

private slots:
    void openAddress();
    void openHome();
    void newTab();
    void closeTab(int index);
    void currentTabChanged(int index);
    void toggleLogs();
    void reloadNode();
    void startNode();
    void stopNode();
    void refreshStatus();
    void bookmarkCurrent();

private:
    void buildUi();
    void buildToolbar();
    void buildDock();
    void registerBrowserCallbacks();
    void maybeShowPermissionPrompt();
    void navigateTo(const QString &text, bool newTab = false);
    QString normalizeTarget(const QString &raw) const;
    QWebEngineView *currentView() const;
    QWebEngineView *createTab(const QUrl &initialUrl);
    void appendLog(const QString &line);

    VX6Backend *m_backend;
    QWebEngineProfile *m_profile;
    QTabWidget *m_tabs;
    QLineEdit *m_address;
    QTextEdit *m_logView;
    QDockWidget *m_logDock;
    QListWidget *m_shortcuts;
};
