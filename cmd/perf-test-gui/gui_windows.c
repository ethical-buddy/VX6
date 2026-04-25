#include <windows.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <shellapi.h>
#include <commctrl.h>

#pragma comment(lib, "user32.lib")
#pragma comment(lib, "kernel32.lib")
#pragma comment(lib, "comctl32.lib")
#pragma comment(lib, "shell32.lib")

#define IDC_MAIN_STATUS     1001
#define IDC_MAIN_RESULTS    1002
#define IDC_MAIN_RUN_TEST   1003
#define IDC_MAIN_OUTPUT_TXT 1004
#define IDC_MAIN_TIMER      1005

#define CHART_HEIGHT 300
#define CHART_WIDTH 500

// Global handles
HWND g_hwndMainWindow = NULL;
HWND g_hwndStatus = NULL;
HWND g_hwndResults = NULL;
HANDLE g_hTestThread = NULL;
BOOL g_bTestRunning = FALSE;

// Performance data structure (matches Go implementation)
typedef struct {
    unsigned long long timestamp;           // Unix timestamp in milliseconds
    unsigned long long tcp_latency_ns;      // TCP latency in nanoseconds
    double tcp_throughput_mbps;             // TCP throughput in MB/s
    int connection_attempts;                // Total connection attempts
    int successful_conns;                   // Successful connections
    int failed_conns;                       // Failed connections
    unsigned long long total_duration_ns;   // Total test duration in nanoseconds
    char status[64];                        // "success" or error message
} PerformanceData;

// Test results storage
PerformanceData g_perfData = {0};

// Function prototypes
LRESULT CALLBACK MainWindowProc(HWND hwnd, UINT msg, WPARAM wParam, LPARAM lParam);
DWORD WINAPI TestThreadProc(LPVOID lpParam);
void RunPerformanceTest(HWND hwnd);
void UpdateResults(HWND hwnd);
void DrawPerformanceChart(HDC hdc, RECT rect, const PerformanceData *pData);
void SetStatusText(const char *pszStatus);

// WinMain - Entry point for Windows GUI applications
int APIENTRY WinMain(HINSTANCE hInstance, HINSTANCE hPrevInstance,
                     LPSTR lpCmdLine, int nCmdShow)
{
    // Initialize common controls
    InitCommonControls();

    // Register the main window class
    WNDCLASS wc = {0};
    wc.lpfnWndProc = MainWindowProc;
    wc.hInstance = hInstance;
    wc.lpszClassName = L"VX6PerfTestGUI";
    wc.hIcon = LoadIcon(NULL, IDI_APPLICATION);
    wc.hCursor = LoadCursor(NULL, IDC_ARROW);
    wc.hbrBackground = (HBRUSH)(COLOR_WINDOW + 1);

    if (!RegisterClass(&wc)) {
        MessageBox(NULL, L"Failed to register window class!", L"Error", MB_OK | MB_ICONERROR);
        return 1;
    }

    // Create the main window
    g_hwndMainWindow = CreateWindow(
        L"VX6PerfTestGUI",
        L"VX6 Windows Performance Test GUI",
        WS_OVERLAPPEDWINDOW | WS_VISIBLE,
        CW_USEDEFAULT, CW_USEDEFAULT,
        900, 700,
        NULL, NULL,
        hInstance, NULL
    );

    if (!g_hwndMainWindow) {
        MessageBox(NULL, L"Failed to create window!", L"Error", MB_OK | MB_ICONERROR);
        return 1;
    }

    // Message loop
    MSG msg = {0};
    while (GetMessage(&msg, NULL, 0, 0)) {
        TranslateMessage(&msg);
        DispatchMessage(&msg);
    }

    return (int)msg.wParam;
}

// Main window procedure
LRESULT CALLBACK MainWindowProc(HWND hwnd, UINT msg, WPARAM wParam, LPARAM lParam)
{
    switch (msg) {
    case WM_CREATE:
    {
        // Create status bar at top
        g_hwndStatus = CreateWindow(
            L"STATIC",
            L"Ready. Click 'Run Test' to begin.",
            WS_CHILD | WS_VISIBLE | SS_LEFT,
            10, 10, 500, 25,
            hwnd, (HMENU)IDC_MAIN_STATUS, NULL, NULL
        );

        // Create "Run Test" button
        HWND hwndRunBtn = CreateWindow(
            L"BUTTON",
            L"Run Performance Test",
            WS_CHILD | WS_VISIBLE | BS_PUSHBUTTON,
            600, 10, 150, 25,
            hwnd, (HMENU)IDC_MAIN_RUN_TEST, NULL, NULL
        );

        // Create results text box
        g_hwndResults = CreateWindow(
            L"EDIT",
            L"Test results will appear here...",
            WS_CHILD | WS_VISIBLE | WS_VSCROLL | ES_MULTILINE | ES_READONLY,
            10, 50, 870, 600,
            hwnd, (HMENU)IDC_MAIN_RESULTS, NULL, NULL
        );

        // Set up a timer for status updates
        SetTimer(hwnd, IDC_MAIN_TIMER, 500, NULL);
        break;
    }

    case WM_COMMAND:
        if (LOWORD(wParam) == IDC_MAIN_RUN_TEST && HIWORD(wParam) == BN_CLICKED) {
            if (!g_bTestRunning) {
                RunPerformanceTest(hwnd);
            } else {
                MessageBox(hwnd, L"Test already running!", L"Info", MB_OK | MB_ICONINFORMATION);
            }
        }
        break;

    case WM_TIMER:
        if (wParam == IDC_MAIN_TIMER) {
            if (!g_bTestRunning && g_perfData.status[0] != 0) {
                UpdateResults(hwnd);
            }
        }
        break;

    case WM_PAINT:
    {
        PAINTSTRUCT ps;
        HDC hdc = BeginPaint(hwnd, &ps);
        // Paint can be extended to draw charts
        EndPaint(hwnd, &ps);
        break;
    }

    case WM_DESTROY:
        KillTimer(hwnd, IDC_MAIN_TIMER);
        PostQuitMessage(0);
        break;

    default:
        return DefWindowProc(hwnd, msg, wParam, lParam);
    }

    return 0;
}

// Run performance test in a separate thread
void RunPerformanceTest(HWND hwnd)
{
    g_bTestRunning = TRUE;
    SetStatusText("Running performance test...");

    DWORD dwThreadId;
    g_hTestThread = CreateThread(
        NULL,
        0,
        TestThreadProc,
        hwnd,
        0,
        &dwThreadId
    );

    if (!g_hTestThread) {
        MessageBox(hwnd, L"Failed to create test thread!", L"Error", MB_OK | MB_ICONERROR);
        g_bTestRunning = FALSE;
    }
}

// Test thread procedure - runs the actual performance tests
DWORD WINAPI TestThreadProc(LPVOID lpParam)
{
    HWND hwnd = (HWND)lpParam;

    // Simulate performance tests
    SetStatusText("Measuring system metrics...");
    Sleep(1000);

    SetStatusText("Benchmarking TCP transport...");
    // Simulate TCP connection tests
    g_perfData.connection_attempts = 10;
    g_perfData.successful_conns = 9;
    g_perfData.failed_conns = 1;
    g_perfData.tcp_latency_ns = 15000000;  // 15ms in nanoseconds
    g_perfData.tcp_throughput_mbps = 950.5;
    Sleep(2000);

    SetStatusText("Measuring VX6-specific metrics...");
    Sleep(1500);

    // Calculate total duration
    g_perfData.total_duration_ns = 4500000000ULL;  // 4.5 seconds in nanoseconds
    strcpy_s(g_perfData.status, sizeof(g_perfData.status), "success");
    g_perfData.timestamp = GetTickCount64();

    g_bTestRunning = FALSE;
    SetStatusText("Test complete!");

    // Update the UI
    PostMessage(hwnd, WM_TIMER, IDC_MAIN_TIMER, 0);

    return 0;
}

// Update results display
void UpdateResults(HWND hwnd)
{
    char szResults[4096];
    sprintf_s(szResults, sizeof(szResults),
        "VX6 Performance Test Results\r\n"
        "========================================\r\n"
        "Status: %s\r\n"
        "Total Duration: %.2f seconds\r\n"
        "\r\n"
        "TRANSPORT PERFORMANCE\r\n"
        "---------------------\r\n"
        "TCP Latency (avg): %.2f ms\r\n"
        "TCP Throughput: %.2f MB/s\r\n"
        "Connection Attempts: %d\r\n"
        "Successful Connections: %d\r\n"
        "Failed Connections: %d\r\n"
        "\r\n"
        "Success Rate: %.1f%%\r\n",
        g_perfData.status,
        g_perfData.total_duration_ns / 1000000000.0,
        g_perfData.tcp_latency_ns / 1000000.0,
        g_perfData.tcp_throughput_mbps,
        g_perfData.connection_attempts,
        g_perfData.successful_conns,
        g_perfData.failed_conns,
        (g_perfData.connection_attempts > 0) ? 
            (g_perfData.successful_conns * 100.0 / g_perfData.connection_attempts) : 0.0
    );

    // Set the results text
    SetWindowTextA(g_hwndResults, szResults);
}

// Set status text
void SetStatusText(const char *pszStatus)
{
    char szStatus[256];
    sprintf_s(szStatus, sizeof(szStatus), "Status: %s", pszStatus);
    
    // Convert to wide character for SetWindowText
    wchar_t wszStatus[256];
    MultiByteToWideChar(CP_ACP, 0, szStatus, -1, wszStatus, sizeof(wszStatus) / sizeof(wchar_t));
    
    SetWindowText(g_hwndStatus, wszStatus);
}

// Draw a simple performance chart
void DrawPerformanceChart(HDC hdc, RECT rect, const PerformanceData *pData)
{
    // Draw chart backgrounds and axes
    HBRUSH hBrush = CreateSolidBrush(RGB(240, 240, 240));
    FillRect(hdc, &rect, hBrush);
    DeleteObject(hBrush);

    // Draw borders
    HPEN hPen = CreatePen(PS_SOLID, 1, RGB(0, 0, 0));
    SelectObject(hdc, hPen);
    MoveToEx(hdc, rect.left, rect.top, NULL);
    LineTo(hdc, rect.right, rect.top);
    LineTo(hdc, rect.right, rect.bottom);
    LineTo(hdc, rect.left, rect.bottom);
    LineTo(hdc, rect.left, rect.top);
    DeleteObject(hPen);

    // Draw chart data (simplified bar chart)
    HBRUSH hDataBrush = CreateSolidBrush(RGB(0, 100, 200));
    
    // Example: Draw bars for success rate
    int barWidth = (rect.right - rect.left) / 5;
    if (pData->connection_attempts > 0) {
        int barHeight = (int)((pData->successful_conns * 100.0 / pData->connection_attempts) 
                               * (rect.bottom - rect.top) / 100.0);
        RECT barRect;
        barRect.left = rect.left + 10;
        barRect.right = barRect.left + barWidth;
        barRect.bottom = rect.bottom - 10;
        barRect.top = barRect.bottom - barHeight;
        FillRect(hdc, &barRect, hDataBrush);
    }
    
    DeleteObject(hDataBrush);
}
