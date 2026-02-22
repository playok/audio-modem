// Audio Modem Web Client

let ws = null;
let currentMode = 'send';
let selectedFile = null;

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    connectWebSocket();
    setupDragDrop();
});

// WebSocket Connection
function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${protocol}//${window.location.host}/ws`);

    ws.onopen = () => {
        addLog('info', 'WebSocket 연결됨');
    };

    ws.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        handleWSMessage(msg);
    };

    ws.onclose = () => {
        addLog('warn', 'WebSocket 연결 끊김. 3초 후 재연결...');
        setTimeout(connectWebSocket, 3000);
    };

    ws.onerror = () => {
        addLog('error', 'WebSocket 오류');
    };
}

function handleWSMessage(msg) {
    switch (msg.type) {
        case 'progress':
            updateProgress(msg.payload);
            break;
        case 'status':
            updateStatus(msg.payload.status, msg.payload.message);
            break;
        case 'log':
            addLog(msg.payload.level, msg.payload.message);
            break;
    }
}

// Mode Selection
function setMode(mode) {
    currentMode = mode;
    document.getElementById('btn-send-mode').classList.toggle('active', mode === 'send');
    document.getElementById('btn-recv-mode').classList.toggle('active', mode === 'receive');
    document.getElementById('send-panel').style.display = mode === 'send' ? 'block' : 'none';
    document.getElementById('receive-panel').style.display = mode === 'receive' ? 'block' : 'none';
}

// File Selection
function handleFileSelect(event) {
    const file = event.target.files[0];
    if (!file) return;

    selectedFile = file;
    const info = document.getElementById('file-info');
    info.textContent = `${file.name} (${formatSize(file.size)})`;
    document.getElementById('btn-send').disabled = false;
    addLog('info', `파일 선택: ${file.name} (${formatSize(file.size)})`);
}

function setupDragDrop() {
    const area = document.getElementById('upload-area');
    if (!area) return;

    area.addEventListener('dragover', (e) => {
        e.preventDefault();
        area.style.borderColor = '#00d4ff';
    });

    area.addEventListener('dragleave', () => {
        area.style.borderColor = '#2a2a4a';
    });

    area.addEventListener('drop', (e) => {
        e.preventDefault();
        area.style.borderColor = '#2a2a4a';
        const file = e.dataTransfer.files[0];
        if (file) {
            selectedFile = file;
            document.getElementById('file-info').textContent = `${file.name} (${formatSize(file.size)})`;
            document.getElementById('btn-send').disabled = false;
            addLog('info', `파일 선택: ${file.name} (${formatSize(file.size)})`);
        }
    });
}

// Send
async function startSend() {
    if (!selectedFile) return;

    const btn = document.getElementById('btn-send');
    btn.disabled = true;
    btn.textContent = '업로드 중...';

    try {
        // Upload file first
        const formData = new FormData();
        formData.append('file', selectedFile);

        const uploadRes = await fetch('/api/upload', { method: 'POST', body: formData });
        if (!uploadRes.ok) throw new Error('Upload failed');

        addLog('info', '파일 업로드 완료, 전송 시작...');
        showProgress();

        // Start sending
        const modulation = document.getElementById('modulation').value;
        const sendRes = await fetch('/api/send', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ filename: selectedFile.name, modulation })
        });

        if (!sendRes.ok) throw new Error('Send start failed');
        addLog('info', `전송 시작 (${modulation})`);
    } catch (err) {
        addLog('error', `오류: ${err.message}`);
        btn.disabled = false;
        btn.textContent = '전송 시작';
    }
}

// Receive
async function startReceive() {
    const btn = document.getElementById('btn-receive');
    btn.disabled = true;
    btn.textContent = '수신 대기 중...';

    try {
        showProgress();
        const modulation = document.getElementById('modulation').value;
        const res = await fetch('/api/receive/start', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ modulation })
        });

        if (!res.ok) throw new Error('Receive start failed');
        addLog('info', `수신 대기 시작 (${modulation})`);
    } catch (err) {
        addLog('error', `오류: ${err.message}`);
        btn.disabled = false;
        btn.textContent = '수신 대기';
    }
}

// Progress
function showProgress() {
    document.getElementById('progress-panel').style.display = 'block';
}

function updateProgress(payload) {
    const fill = document.getElementById('progress-fill');
    const percent = document.getElementById('progress-percent');
    const bytes = document.getElementById('progress-bytes');
    const message = document.getElementById('progress-message');

    const pct = Math.round(payload.progress * 100);
    fill.style.width = pct + '%';
    percent.textContent = pct + '%';

    if (payload.totalBytes > 0) {
        bytes.textContent = `${formatSize(payload.bytesSent)} / ${formatSize(payload.totalBytes)}`;
    }

    message.textContent = payload.message || '';
    updateStatus(payload.status, payload.message);
}

function updateStatus(status, message) {
    const el = document.getElementById('connection-status');
    const text = document.getElementById('status-text');

    el.className = 'status ' + status;

    const statusNames = {
        disconnected: '연결 대기',
        connecting: '연결 중...',
        connected: '연결됨',
        transferring: '전송 중',
        completed: '완료',
        error: '오류'
    };

    text.textContent = message || statusNames[status] || status;

    if (status === 'completed') {
        resetButtons();
        addLog('success', message || '전송 완료!');
    } else if (status === 'error') {
        resetButtons();
        addLog('error', message || '오류 발생');
    }
}

function resetButtons() {
    const sendBtn = document.getElementById('btn-send');
    const recvBtn = document.getElementById('btn-receive');
    if (sendBtn) {
        sendBtn.disabled = !selectedFile;
        sendBtn.textContent = '전송 시작';
    }
    if (recvBtn) {
        recvBtn.disabled = false;
        recvBtn.textContent = '수신 대기';
    }
}

// Log
function addLog(level, message) {
    const container = document.getElementById('log-container');
    const entry = document.createElement('div');
    entry.className = `log-entry ${level}`;
    const time = new Date().toLocaleTimeString();
    entry.textContent = `[${time}] ${message}`;
    container.appendChild(entry);
    container.scrollTop = container.scrollHeight;

    // Limit log entries
    while (container.children.length > 100) {
        container.removeChild(container.firstChild);
    }
}

// Utilities
function formatSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}
