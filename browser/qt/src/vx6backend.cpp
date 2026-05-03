#include "vx6backend.h"

#include <QCoreApplication>
#include <QDir>
#include <QFileInfo>
#include <QProcess>
#include <QProcessEnvironment>
#include <QStandardPaths>

VX6Backend::VX6Backend(QString vx6Binary, QString configPath, QObject *parent)
    : QObject(parent), m_vx6Binary(std::move(vx6Binary)), m_configPath(std::move(configPath))
{
    m_nodeProcess.setProcessChannelMode(QProcess::MergedChannels);
    connect(&m_nodeProcess, &QProcess::readyReadStandardOutput, this, &VX6Backend::appendProcessOutput);
    connect(&m_nodeProcess, &QProcess::readyReadStandardError, this, &VX6Backend::appendProcessOutput);
    connect(&m_nodeProcess, &QProcess::finished, this, [this](int, QProcess::ExitStatus) {
        emit logLine(QStringLiteral("vx6 node stopped"));
        updateNodeState();
    });
}

QString VX6Backend::vx6Binary() const
{
    return m_vx6Binary;
}

QString VX6Backend::configPath() const
{
    return m_configPath;
}

void VX6Backend::setVx6Binary(const QString &path)
{
    m_vx6Binary = path;
}

void VX6Backend::setConfigPath(const QString &path)
{
    m_configPath = path;
}

bool VX6Backend::needsPermissionPrompt() const
{
#if defined(Q_OS_WIN) || defined(Q_OS_MACOS)
    return true;
#else
    return false;
#endif
}

QString VX6Backend::resolveBinaryPath() const
{
    if (!m_vx6Binary.trimmed().isEmpty()) {
        return m_vx6Binary.trimmed();
    }

    const QString appDir = QCoreApplication::applicationDirPath();
    const QStringList candidates = {
#if defined(Q_OS_WIN)
        QDir(appDir).filePath("vx6.exe"),
        QDir(appDir).filePath("vx6"),
#else
        QDir(appDir).filePath("vx6"),
#endif
    };

    for (const QString &candidate : candidates) {
        if (QFileInfo::exists(candidate)) {
            return candidate;
        }
    }
    const QString fromPath = QStandardPaths::findExecutable(QStringLiteral("vx6"));
    if (!fromPath.isEmpty()) {
        return fromPath;
    }
    return QStringLiteral("vx6");
}

QString VX6Backend::resolveConfigPath() const
{
    if (!m_configPath.trimmed().isEmpty()) {
        return QDir::cleanPath(m_configPath.trimmed());
    }

    const QString base = QStandardPaths::writableLocation(QStandardPaths::AppConfigLocation);
    if (!base.isEmpty()) {
        return QDir(base).filePath("config.json");
    }
    return QStringLiteral("config.json");
}

QString VX6Backend::runVX6(const QStringList &args, bool *ok) const
{
    QProcess proc;
    proc.setProgram(resolveBinaryPath());
    proc.setArguments(args);

    QProcessEnvironment env = QProcessEnvironment::systemEnvironment();
    const QString cfg = resolveConfigPath();
    if (!cfg.isEmpty()) {
        env.insert(QStringLiteral("VX6_CONFIG_PATH"), cfg);
    }
    proc.setProcessEnvironment(env);
    proc.start();
    if (!proc.waitForStarted(5000)) {
        if (ok) {
            *ok = false;
        }
        const QString msg = QStringLiteral("failed to start vx6 binary: %1").arg(resolveBinaryPath());
        emit logLine(msg);
        return msg;
    }
    if (!proc.waitForFinished(120000)) {
        proc.kill();
        proc.waitForFinished(2000);
        if (ok) {
            *ok = false;
        }
        const QString msg = QStringLiteral("vx6 command timed out: %1").arg(args.join(' '));
        emit logLine(msg);
        return msg;
    }

    const QString out = QString::fromUtf8(proc.readAllStandardOutput());
    const QString err = QString::fromUtf8(proc.readAllStandardError());
    const bool success = proc.exitStatus() == QProcess::NormalExit && proc.exitCode() == 0;
    if (ok) {
        *ok = success;
    }

    QString combined = out;
    if (!err.trimmed().isEmpty()) {
        if (!combined.endsWith('\n') && !combined.isEmpty()) {
            combined += '\n';
        }
        combined += err;
    }
    const QString line = QStringLiteral("[%1] %2").arg(success ? "ok" : "fail", args.join(' '));
    emit logLine(line);
    if (!success && combined.trimmed().isEmpty()) {
        combined = QStringLiteral("vx6 command failed with exit code %1").arg(proc.exitCode());
    }
    return combined;
}

QString VX6Backend::browserHintBlock() const
{
    return QStringLiteral(
        "<div class=\"hint\"><strong>Browser mode:</strong> "
        "VX6 pages are opened through <code>vx6://</code>. "
        "Standard <code>http://</code> and <code>https://</code> pages stay available.</div>");
}

QString VX6Backend::osNoticeBlock() const
{
#if defined(Q_OS_WIN)
    return QStringLiteral(
        "<div class=\"notice warn\"><strong>Windows start-up:</strong> "
        "allow firewall/admin setup during first launch so service discovery works later without prompts.</div>");
#elif defined(Q_OS_MACOS)
    return QStringLiteral(
        "<div class=\"notice warn\"><strong>macOS start-up:</strong> "
        "grant network and firewall permissions during first launch so VX6 can publish and connect cleanly.</div>");
#else
    return QStringLiteral(
        "<div class=\"notice ok\"><strong>Linux/BSD start-up:</strong> "
        "VX6 runs with the same backend; platform-specific firewall prompts are not forced here.</div>");
#endif
}

QString VX6Backend::dashboardCard(const QString &href, const QString &title, const QString &description, const QString &accent) const
{
    return QStringLiteral(
        "<a class=\"card\" href=\"%1\" style=\"--accent:%4\">"
        "<span class=\"card-title\">%2</span>"
        "<span class=\"card-desc\">%3</span>"
        "</a>")
        .arg(href, title, description, accent);
}

QString VX6Backend::commandBlock(const QString &output) const
{
    return QStringLiteral("<pre class=\"output\">%1</pre>").arg(output.toHtmlEscaped());
}

QString VX6Backend::makePageShell(const QString &title, const QString &headline, const QString &body, const QString &accent) const
{
    QString page = QStringLiteral(R"HTML(
<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{TITLE}}</title>
<style>
:root {
  color-scheme: dark;
  --text: #eef4ff;
  --muted: #a8b3ca;
  --accent: {{ACCENT}};
}
* { box-sizing: border-box; }
body {
  margin: 0;
  font-family: Inter, Segoe UI, Arial, sans-serif;
  background:
    radial-gradient(circle at top left, rgba(27, 111, 255, 0.20), transparent 28%),
    radial-gradient(circle at top right, rgba(6, 214, 160, 0.18), transparent 26%),
    linear-gradient(180deg, #07111d 0%, #0c1426 100%);
  color: var(--text);
}
.wrap { padding: 28px; max-width: 1240px; margin: 0 auto; }
.banner, .panel, .notice, .hint, .card, .output {
  border-radius: 20px;
  border: 1px solid rgba(255,255,255,0.08);
  background: rgba(16, 27, 49, 0.84);
  box-shadow: 0 12px 40px rgba(0,0,0,0.22);
}
.banner { padding: 24px; background: linear-gradient(145deg, rgba(16,27,49,.95), rgba(19,32,58,.86)); }
.panel { padding: 20px; margin-top: 18px; }
.title { margin: 0 0 10px; font-size: 42px; letter-spacing: 0.02em; }
.subtitle { margin: 0 0 18px; color: var(--muted); line-height: 1.6; }
.notice, .hint { padding: 16px 18px; margin: 0 0 14px; }
.notice.ok { border-color: rgba(6, 214, 160, 0.30); }
.notice.warn { border-color: rgba(255, 194, 102, 0.34); }
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 14px;
}
.card {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-height: 120px;
  padding: 18px;
  color: var(--text);
  text-decoration: none;
  border-left: 6px solid var(--accent);
  background: linear-gradient(180deg, rgba(20,31,54,.96), rgba(14,22,40,.96));
}
.card:hover { transform: translateY(-2px); }
.card-title { font-size: 18px; font-weight: 700; margin-bottom: 8px; }
.card-desc { color: var(--muted); line-height: 1.45; }
.section { margin-top: 22px; }
.section h2 { margin: 0 0 12px; font-size: 22px; }
.output {
  padding: 18px;
  overflow: auto;
  white-space: pre-wrap;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 13px;
  line-height: 1.55;
}
.footer { margin-top: 24px; color: var(--muted); font-size: 13px; }
code { background: rgba(255,255,255,0.08); padding: 2px 6px; border-radius: 6px; }
a { color: #79b8ff; }
</style>
</head>
<body>
<div class="wrap">
  <div class="banner">
    <h1 class="title">{{TITLE}}</h1>
    <p class="subtitle">{{HEADLINE}}</p>
    {{HINT}}
    {{NOTICE}}
  </div>
  {{BODY}}
  <div class="footer">
    Shell accent: <span style="color:{{ACCENT}};">{{ACCENT}}</span> • backed by the current VX6 binary • no profile switching
  </div>
</div>
</body>
</html>
)HTML");

    page.replace("{{TITLE}}", title.toHtmlEscaped());
    page.replace("{{HEADLINE}}", headline.toHtmlEscaped());
    page.replace("{{ACCENT}}", accent.toHtmlEscaped());
    page.replace("{{HINT}}", browserHintBlock());
    page.replace("{{NOTICE}}", osNoticeBlock());
    page.replace("{{BODY}}", body);
    return page;
}

QString VX6Backend::homePageHtml() const
{
    bool ok = false;
    const QString status = runVX6(QStringList{"status"}, &ok);
    const QString dht = runVX6(QStringList{"debug", "dht-status"}, &ok);
    const QString peers = runVX6(QStringList{"peer"}, &ok);

    QString cards;
    cards += dashboardCard("vx6://status", "Status", "Show live runtime status and current node health.", "#6ea8ff");
    cards += dashboardCard("vx6://dht", "DHT", "Open lookup, replication, and resolver health.", "#23c18f");
    cards += dashboardCard("vx6://registry", "Registry", "Inspect the discovery registry snapshot.", "#ffd166");
    cards += dashboardCard("vx6://services", "Services", "View local configured services.", "#f78c6b");
    cards += dashboardCard("vx6://peers", "Peers", "View local peers and sync targets.", "#8f7cff");
    cards += dashboardCard("vx6://identity", "Identity", "Show the node key and identity details.", "#4dd0e1");
    cards += dashboardCard("vx6://permissions", "Permissions", "First-run firewall/admin guidance.", "#ff7eb6");
    cards += dashboardCard("vx6://service/example", "Service Lookup", "Look up a service by name.", "#9bde7b");
    cards += dashboardCard("vx6://node/example", "Node Lookup", "Look up a node by name.", "#ffb86b");
    cards += dashboardCard("vx6://key/example", "Raw Key", "Inspect a raw DHT key.", "#c792ea");

    QString body;
    body += QStringLiteral("<div class=\"grid\">%1</div>").arg(cards);
    body += QStringLiteral("<div class=\"section\"><h2>Live Status Snapshot</h2>%1</div>").arg(commandBlock(status));
    body += QStringLiteral("<div class=\"section\"><h2>DHT Snapshot</h2>%1</div>").arg(commandBlock(dht));
    body += QStringLiteral("<div class=\"section\"><h2>Peer Snapshot</h2>%1</div>").arg(commandBlock(peers));
    body += QStringLiteral(
        "<div class=\"section\"><h2>Shortcuts</h2>"
        "<div class=\"hint\">Use <code>vx6://status</code>, <code>vx6://dht</code>, <code>vx6://services</code>, "
        "<code>vx6://peers</code>, and <code>vx6://identity</code> as your default home actions.</div>"
        "</div>");

    return makePageShell("VX6 Home", "One system. One key. One VX6 runtime.", body, "#6ea8ff");
}

QString VX6Backend::statusPageHtml() const
{
    bool ok = false;
    const QString output = runVX6(QStringList{"status"}, &ok);
    const QString body = QStringLiteral(
        "<div class=\"hint\">Live runtime status from the current VX6 binary.</div>"
        "<div class=\"section\"><h2>Status Output</h2>%1</div>")
        .arg(commandBlock(output));
    return makePageShell("VX6 Status", "Live runtime status", body, "#6ea8ff");
}

QString VX6Backend::dhtPageHtml() const
{
    bool ok = false;
    const QString output = runVX6(QStringList{"debug", "dht-status"}, &ok);
    const QString body = QStringLiteral(
        "<div class=\"hint\">Lookup health, ASN diversity, replicas, and refresh state.</div>"
        "<div class=\"section\"><h2>DHT Output</h2>%1</div>")
        .arg(commandBlock(output));
    return makePageShell("VX6 DHT", "DHT health and lookup state", body, "#23c18f");
}

QString VX6Backend::registryPageHtml() const
{
    bool ok = false;
    const QString output = runVX6(QStringList{"debug", "registry"}, &ok);
    const QString body = QStringLiteral(
        "<div class=\"hint\">Discovery registry snapshot from the current node.</div>"
        "<div class=\"section\"><h2>Registry Output</h2>%1</div>")
        .arg(commandBlock(output));
    return makePageShell("VX6 Registry", "Discovery registry", body, "#ffd166");
}

QString VX6Backend::servicesPageHtml() const
{
    bool ok = false;
    const QString output = runVX6(QStringList{"service"}, &ok);
    const QString body = QStringLiteral(
        "<div class=\"hint\">Local configured services. Public, private, and hidden services are all surfaced here.</div>"
        "<div class=\"section\"><h2>Services Output</h2>%1</div>")
        .arg(commandBlock(output));
    return makePageShell("VX6 Services", "Local services", body, "#f78c6b");
}

QString VX6Backend::peersPageHtml() const
{
    bool ok = false;
    const QString output = runVX6(QStringList{"peer"}, &ok);
    const QString body = QStringLiteral(
        "<div class=\"hint\">Known peers are the seed list used at startup and for later sync.</div>"
        "<div class=\"section\"><h2>Peer Output</h2>%1</div>")
        .arg(commandBlock(output));
    return makePageShell("VX6 Peers", "Peer directory", body, "#8f7cff");
}

QString VX6Backend::identityPageHtml() const
{
    bool ok = false;
    const QString output = runVX6(QStringList{"identity"}, &ok);
    const QString body = QStringLiteral(
        "<div class=\"hint\">One system, one key. The display name can change; the key stays fixed.</div>"
        "<div class=\"section\"><h2>Identity Output</h2>%1</div>")
        .arg(commandBlock(output));
    return makePageShell("VX6 Identity", "Identity and key details", body, "#4dd0e1");
}

QString VX6Backend::lookupPageHtml(const QString &title, const QStringList &args, const QString &subtitle) const
{
    bool ok = false;
    const QString output = runVX6(args, &ok);
    const QString body = QStringLiteral(
        "<div class=\"hint\">%1</div>"
        "<div class=\"section\"><h2>%2 Output</h2>%3</div>")
        .arg(subtitle.toHtmlEscaped(), title.toHtmlEscaped(), commandBlock(output));
    return makePageShell(title, subtitle, body, "#c792ea");
}

bool VX6Backend::nodeRunning() const
{
    return m_nodeProcess.state() != QProcess::NotRunning;
}

QString VX6Backend::startNode()
{
    if (nodeRunning()) {
        return QStringLiteral("vx6 node already running");
    }

    m_nodeProcess.setProgram(resolveBinaryPath());
    m_nodeProcess.setArguments({"node"});
    QProcessEnvironment env = QProcessEnvironment::systemEnvironment();
    const QString cfg = resolveConfigPath();
    if (!cfg.isEmpty()) {
        env.insert(QStringLiteral("VX6_CONFIG_PATH"), cfg);
    }
    m_nodeProcess.setProcessEnvironment(env);
    m_nodeProcess.start();
    if (!m_nodeProcess.waitForStarted(5000)) {
        return QStringLiteral("failed to start vx6 node: %1").arg(resolveBinaryPath());
    }

    updateNodeState();
    emit logLine(QStringLiteral("vx6 node started"));
    return QStringLiteral("vx6 node started");
}

QString VX6Backend::stopNode()
{
    if (!nodeRunning()) {
        return QStringLiteral("vx6 node is not running");
    }

    m_nodeProcess.terminate();
    if (!m_nodeProcess.waitForFinished(3000)) {
        m_nodeProcess.kill();
        m_nodeProcess.waitForFinished(2000);
    }
    updateNodeState();
    return QStringLiteral("vx6 node stopped");
}

void VX6Backend::appendProcessOutput()
{
    const QString out = QString::fromUtf8(m_nodeProcess.readAllStandardOutput());
    const QString err = QString::fromUtf8(m_nodeProcess.readAllStandardError());
    const QString text = (out + err).trimmed();
    if (!text.isEmpty()) {
        emit logLine(text);
    }
}

void VX6Backend::updateNodeState()
{
    emit logLine(QStringLiteral("vx6 node state: %1").arg(nodeRunning() ? "running" : "stopped"));
}

QString VX6Backend::permissionPromptHtml() const
{
    const QString body = QStringLiteral(
        "<div class=\"notice warn\"><strong>First run permissions.</strong> "
        "Allow firewall/admin setup on Windows or macOS now so node publishing and discovery do not fail later.</div>"
        "<div class=\"section\"><h2>What to allow</h2>"
        "<div class=\"hint\">"
        "<ul>"
        "<li>Allow VX6 network access for the browser shell and node runtime.</li>"
        "<li>Allow the installer or first-run elevated prompt if your OS requests it.</li>"
        "<li>Keep the same binary path after install so firewall rules stay valid.</li>"
        "</ul>"
        "</div></div>");
    return makePageShell("VX6 Permissions", "Startup permissions and firewall guidance", body, "#ff7eb6");
}
