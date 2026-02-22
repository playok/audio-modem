// ============================================================
// Audio Modem — Browser App (No Server Required)
// ============================================================

let audioCtx = null;
let micStream = null;
let isRecording = false;
let recordedChunks = [];
let selectedFile = null;
let selectedFileName = '';
let modulation = 'QPSK';

// --- Init ---
document.addEventListener('DOMContentLoaded', () => {
    document.getElementById('modulation').addEventListener('change', e => {
        modulation = e.target.value;
        addLog('info', `변조 방식: ${modulation}`);
    });
});

function getModemParams(mod) {
    if (mod === 'BPSK-ACOUSTIC') return { config: 'acoustic', modName: 'BPSK' };
    if (mod === '16-QAM') return { config: 'standard', modName: 'QAM16' };
    return { config: 'standard', modName: 'QPSK' };
}

function getAudioContext() {
    if (!audioCtx || audioCtx.state === 'closed') {
        audioCtx = new (window.AudioContext || window.webkitAudioContext)({ sampleRate: 44100 });
    }
    if (audioCtx.state === 'suspended') audioCtx.resume();
    return audioCtx;
}

// --- Mode ---
function setMode(mode) {
    document.getElementById('btn-send-mode').classList.toggle('active', mode === 'send');
    document.getElementById('btn-recv-mode').classList.toggle('active', mode === 'receive');
    document.getElementById('send-panel').style.display = mode === 'send' ? 'block' : 'none';
    document.getElementById('receive-panel').style.display = mode === 'receive' ? 'block' : 'none';
}

// --- File Selection ---
function handleFileSelect(event) {
    const file = event.target.files[0];
    if (!file) return;
    selectedFile = file;
    selectedFileName = file.name;
    document.getElementById('file-info').textContent = `${file.name} (${formatSize(file.size)})`;
    document.getElementById('btn-send').disabled = false;
    addLog('info', `파일 선택: ${file.name} (${formatSize(file.size)})`);
}

// --- Send ---
async function startSend() {
    if (!selectedFile) return;

    const btn = document.getElementById('btn-send');
    btn.disabled = true;
    btn.textContent = '변조 중...';
    showProgress();
    updateProgress(0, '파일 읽는 중...');

    try {
        const arrayBuf = await selectedFile.arrayBuffer();
        const fileData = new Uint8Array(arrayBuf);

        addLog('info', `변조 시작 (${modulation}, ${fileData.length} bytes)`);
        updateProgress(0.1, 'OFDM 변조 중...');

        // Build signal (run in next frame to update UI)
        await sleep(50);
        const { config, modName } = getModemParams(modulation);
        setOFDMConfig(config);
        const result = buildTransmitSignal(fileData, modName, selectedFileName);

        const duration = result.signal.length / 44100;
        addLog('info', `변조 완료: ${result.numSymbols} 심볼, ${duration.toFixed(1)}초`);
        updateProgress(0.3, '오디오 재생 중...');

        // Play signal
        const ctx = getAudioContext();
        const buffer = ctx.createBuffer(1, result.signal.length, 44100);
        buffer.getChannelData(0).set(result.signal);

        const source = ctx.createBufferSource();
        source.buffer = buffer;
        source.connect(ctx.destination);

        source.onended = () => {
            updateProgress(1.0, '전송 완료!');
            addLog('success', `전송 완료: ${selectedFileName} (${formatSize(fileData.length)})`);
            btn.disabled = false;
            btn.textContent = '전송 시작';
        };

        source.start();

        // Progress animation during playback
        const startTime = ctx.currentTime;
        const progressInterval = setInterval(() => {
            const elapsed = ctx.currentTime - startTime;
            const progress = Math.min(0.3 + 0.7 * (elapsed / duration), 0.99);
            updateProgress(progress, `재생 중... ${elapsed.toFixed(1)}s / ${duration.toFixed(1)}s`);
            if (elapsed >= duration) clearInterval(progressInterval);
        }, 200);

    } catch (err) {
        addLog('error', `전송 오류: ${err.message}`);
        btn.disabled = false;
        btn.textContent = '전송 시작';
    }
}

// --- Receive ---
async function startReceive() {
    const btn = document.getElementById('btn-receive');

    if (isRecording) {
        stopReceive();
        return;
    }

    try {
        // Request microphone permission
        micStream = await navigator.mediaDevices.getUserMedia({
            audio: {
                echoCancellation: false,
                noiseSuppression: false,
                autoGainControl: false,
                sampleRate: 44100,
            }
        });
        addLog('info', '마이크 권한 허용됨');
    } catch (err) {
        addLog('error', `마이크 접근 실패: ${err.message}`);
        addLog('warn', 'HTTPS 또는 localhost에서만 마이크를 사용할 수 있습니다.');
        return;
    }

    isRecording = true;
    recordedChunks = [];
    btn.textContent = '수신 중지';
    btn.classList.add('recording');
    showProgress();
    updateProgress(0, '수신 대기 중... (신호를 보내주세요)');
    addLog('info', '수신 대기 시작 — 상대방이 전송을 시작하세요');

    const ctx = getAudioContext();
    const source = ctx.createMediaStreamSource(micStream);

    // Use ScriptProcessorNode for broad compatibility
    const processor = ctx.createScriptProcessor(4096, 1, 1);
    let totalSamples = 0;
    const maxDuration = 120; // 2분 제한

    processor.onaudioprocess = (e) => {
        if (!isRecording) return;
        const input = e.inputBuffer.getChannelData(0);
        recordedChunks.push(new Float32Array(input));
        totalSamples += input.length;

        const seconds = totalSamples / 44100;
        if (seconds % 1 < 0.1) {
            updateProgress(0, `녹음 중... ${seconds.toFixed(0)}초 (${formatSize(totalSamples * 4)})`);
        }

        if (seconds >= maxDuration) {
            stopReceive();
        }
    };

    source.connect(processor);
    processor.connect(ctx.destination);

    // Store references for cleanup
    btn._source = source;
    btn._processor = processor;
}

function stopReceive() {
    isRecording = false;
    const btn = document.getElementById('btn-receive');
    btn.textContent = '수신 대기';
    btn.classList.remove('recording');

    // Cleanup audio nodes
    if (btn._processor) { btn._processor.disconnect(); btn._processor = null; }
    if (btn._source) { btn._source.disconnect(); btn._source = null; }
    if (micStream) { micStream.getTracks().forEach(t => t.stop()); micStream = null; }

    if (recordedChunks.length === 0) {
        addLog('warn', '녹음된 데이터가 없습니다');
        return;
    }

    // Concatenate chunks
    let totalLen = 0;
    for (const c of recordedChunks) totalLen += c.length;
    const signal = new Float32Array(totalLen);
    let off = 0;
    for (const c of recordedChunks) { signal.set(c, off); off += c.length; }
    recordedChunks = [];

    addLog('info', `녹음 완료: ${(totalLen / 44100).toFixed(1)}초, 복조 시작...`);
    updateProgress(0.3, '복조 중...');

    // Decode
    setTimeout(() => {
        try {
            const { config, modName } = getModemParams(modulation);
            setOFDMConfig(config);
            const result = decodeReceivedSignal(signal, modName);

            if (result.error) {
                addLog('error', `복조 실패: ${result.error}`);
                updateProgress(0, `오류: ${result.error}`);
                return;
            }

            if (result.crcValid) {
                addLog('success', `수신 성공! ${result.fileName || 'file'} — CRC 검증 통과 (${formatSize(result.dataLen)})`);
                updateProgress(1.0, `수신 완료: ${result.fileName || 'file'} (${formatSize(result.dataLen)})`);
                offerDownload(result.data, result.fileName || 'received_file');
            } else {
                addLog('error', `CRC 불일치 (expected: ${result.expectedCRC.toString(16)}, got: ${result.actualCRC.toString(16)})`);
                addLog('warn', '데이터가 손상되었을 수 있습니다. 다운로드를 시도합니다.');
                updateProgress(0.9, 'CRC 불일치 — 데이터 손상 가능');
                offerDownload(result.data, (result.fileName || 'received_file') + '.corrupted');
            }
        } catch (err) {
            addLog('error', `복조 오류: ${err.message}`);
            updateProgress(0, `오류: ${err.message}`);
        }
    }, 100);
}

function offerDownload(data, defaultName) {
    const blob = new Blob([data]);
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = defaultName;
    a.textContent = `${defaultName} (${formatSize(data.length)})`;
    a.className = 'download-link';

    const container = document.getElementById('received-files');
    const item = document.createElement('div');
    item.className = 'file-item';
    item.appendChild(a);
    container.appendChild(item);

    addLog('info', '파일 다운로드 링크가 생성되었습니다');
}

// --- Progress ---
function showProgress() {
    document.getElementById('progress-panel').style.display = 'block';
}

function updateProgress(ratio, message) {
    const pct = Math.round(ratio * 100);
    document.getElementById('progress-fill').style.width = pct + '%';
    document.getElementById('progress-percent').textContent = pct + '%';
    document.getElementById('progress-message').textContent = message || '';
}

// --- Log ---
function addLog(level, message) {
    const container = document.getElementById('log-container');
    const entry = document.createElement('div');
    entry.className = `log-entry ${level}`;
    const time = new Date().toLocaleTimeString();
    entry.textContent = `[${time}] ${message}`;
    container.appendChild(entry);
    container.scrollTop = container.scrollHeight;
    while (container.children.length > 100) container.removeChild(container.firstChild);
}

// --- Utilities ---
function formatSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024, sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }
