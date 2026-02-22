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
let fullSignal = null;       // 녹음된 전체 신호 (파형 트리머용)
let levelAnalyser = null;    // 레벨미터용 AnalyserNode

// --- Init ---
document.addEventListener('DOMContentLoaded', () => {
    document.getElementById('modulation').addEventListener('change', e => {
        modulation = e.target.value;
        addLog('info', `변조 방식: ${modulation}`);
        updateModulationInfo();
    });
    document.getElementById('max-duration').addEventListener('change', () => {
        updateModulationInfo();
    });
    updateModulationInfo();
});

function getMaxDuration() {
    return parseInt(document.getElementById('max-duration').value) || 600;
}

function updateModulationInfo() {
    const MAX_DURATION = getMaxDuration();
    const HEADER_BYTES = 15; // nameLen(1) + name(~6) + dataLen(4) + CRC(4)
    const { config, modName, repetition } = getModemParams(modulation);
    const cfg = OFDM_CONFIGS[config];

    // 데이터 서브캐리어 수 계산
    let dataSubs = 0;
    for (let k = cfg.SUB_START; k <= cfg.SUB_END; k++) {
        if (!cfg.PILOTS.includes(k)) dataSubs++;
    }

    const bps = Constellations[modName].bps;
    const bitsPerSymbol = dataSubs * bps;
    const symDuration = cfg.SYMBOL_LEN / cfg.SAMPLE_RATE;
    const isAcoustic = cfg.CP_LEN >= 128;
    const overhead = (isAcoustic ? 1.0 : 0.5) + 3 * symDuration;
    const availTime = MAX_DURATION - overhead;
    const maxSymbols = Math.floor(availTime / symDuration);
    const maxBits = maxSymbols * bitsPerSymbol;
    const maxBytes = Math.floor(maxBits / 8 / repetition) - HEADER_BYTES;
    const speed = maxBytes / availTime;

    const el = document.getElementById('modulation-info');
    const minutes = Math.round(MAX_DURATION / 60);
    el.innerHTML = `최대 수신: <strong style="color:#00d4ff">${formatSize(maxBytes)}</strong> (${minutes}분 녹음) · 속도: ~${formatSize(Math.round(speed))}/s`;
}

function getModemParams(mod) {
    if (mod === 'BPSK-ACOUSTIC') return { config: 'acoustic', modName: 'BPSK', repetition: 1 };
    if (mod === 'BPSK-REPEAT') return { config: 'acoustic', modName: 'BPSK', repetition: 3 };
    if (mod === 'BPSK-NARROW') return { config: 'narrowband', modName: 'BPSK', repetition: 3 };
    if (mod === '16-QAM') return { config: 'standard', modName: 'QAM16', repetition: 1 };
    return { config: 'standard', modName: 'QPSK', repetition: 1 };
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

// --- Receive Mode ---
let receiveMode = 'manual'; // 'manual' or 'streaming'

function setReceiveMode(mode) {
    receiveMode = mode;
    document.getElementById('recv-mode-manual').classList.toggle('active', mode === 'manual');
    document.getElementById('recv-mode-streaming').classList.toggle('active', mode === 'streaming');
    document.getElementById('recv-manual-desc').style.display = mode === 'manual' ? 'block' : 'none';
    document.getElementById('recv-streaming-desc').style.display = mode === 'streaming' ? 'block' : 'none';

    // Hide chunk progress when switching to manual
    if (mode === 'manual') {
        const cp = document.getElementById('chunk-progress');
        if (cp) cp.style.display = 'none';
    }
}

function onReceiveClick() {
    if (receiveMode === 'streaming') {
        startStreamingReceive();
    } else {
        startReceive();
    }
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
const CHUNK_THRESHOLD = 32 * 1024; // 32KB — 이 이하는 레거시, 이상은 청크
let chunkedSendAbort = false;

async function startSend() {
    if (!selectedFile) return;

    const { config, modName, repetition } = getModemParams(modulation);
    setOFDMConfig(config);

    if (selectedFile.size <= CHUNK_THRESHOLD) {
        await startSendLegacy();
    } else {
        await playChunkedFrames();
    }
}

// 기존 단일 프레임 전송 (소규모 파일)
async function startSendLegacy() {
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

        await sleep(50);
        const { config, modName, repetition } = getModemParams(modulation);
        setOFDMConfig(config);
        const result = buildTransmitSignal(fileData, modName, selectedFileName, repetition);

        const duration = result.signal.length / 44100;
        addLog('info', `변조 완료: ${result.numSymbols} 심볼, ${duration.toFixed(1)}초`);
        updateProgress(0.3, '오디오 재생 중...');

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

// --- Chunked Send (대용량 파일, 더블 버퍼링) ---

function getChunkSize(modName) {
    if (modName === 'QAM16') return 4096;
    if (modName === 'QPSK') return 2048;
    return 512; // BPSK
}

async function playChunkedFrames() {
    const btn = document.getElementById('btn-send');
    btn.disabled = true;
    chunkedSendAbort = false;

    const { config, modName, repetition } = getModemParams(modulation);
    setOFDMConfig(config);

    const fileSize = selectedFile.size;
    const chunkSize = getChunkSize(modName);
    const totalChunks = Math.ceil(fileSize / chunkSize);

    addLog('info', `청크 전송 시작: ${selectedFileName} (${formatSize(fileSize)}, ${totalChunks}개 청크, 각 ${formatSize(chunkSize)})`);
    showProgress();
    updateChunkProgressUI(0, totalChunks, 0);

    const ctx = getAudioContext();
    const sendStartTime = Date.now();

    try {
        // 1. 메타데이터 프레임 전송
        btn.textContent = '메타 전송 중...';
        updateProgress(0, '메타데이터 프레임 전송 중...');

        const metaSignal = buildMetadataFrame(totalChunks, fileSize, chunkSize, selectedFileName, modName, repetition);
        await playSignalAsync(ctx, metaSignal);

        if (chunkedSendAbort) { finishChunkedSend(btn, '전송 중단됨'); return; }

        // 2. 데이터 청크 순차 전송 (더블 버퍼링)
        btn.textContent = '전송 중지';
        btn.disabled = false;
        btn.onclick = () => { chunkedSendAbort = true; };

        let nextFrameSignal = null; // 미리 빌드된 다음 프레임

        for (let seq = 0; seq < totalChunks; seq++) {
            if (chunkedSendAbort) break;

            // 현재 청크 프레임 준비 (더블 버퍼에서 가져오거나 새로 빌드)
            let currentSignal;
            if (nextFrameSignal) {
                currentSignal = nextFrameSignal;
                nextFrameSignal = null;
            } else {
                const chunkData = await readFileChunk(selectedFile, seq, chunkSize);
                currentSignal = buildDataChunkFrame(chunkData, seq, modName, repetition);
            }

            // 다음 프레임 미리 빌드 (비동기 시작)
            const nextSeq = seq + 1;
            let nextBuildPromise = null;
            if (nextSeq < totalChunks) {
                nextBuildPromise = readFileChunk(selectedFile, nextSeq, chunkSize).then(
                    data => buildDataChunkFrame(data, nextSeq, modName, repetition)
                );
            }

            // 현재 프레임 재생
            await playSignalAsync(ctx, currentSignal);

            // 다음 프레임 빌드 완료 대기
            if (nextBuildPromise) {
                nextFrameSignal = await nextBuildPromise;
            }

            // 진행률 업데이트
            const elapsed = (Date.now() - sendStartTime) / 1000;
            const progress = (seq + 1) / totalChunks;
            const eta = elapsed / progress * (1 - progress);
            updateChunkProgressUI(seq + 1, totalChunks, eta);
            updateProgress(progress, `청크 ${seq + 1}/${totalChunks} 전송 완료 · ETA: ${formatETA(eta)}`);
        }

        if (chunkedSendAbort) {
            finishChunkedSend(btn, '전송이 사용자에 의해 중단되었습니다');
        } else {
            updateProgress(1.0, '전송 완료!');
            addLog('success', `전송 완료: ${selectedFileName} (${formatSize(fileSize)}, ${totalChunks}개 청크)`);
            finishChunkedSend(btn, null);
        }

    } catch (err) {
        addLog('error', `청크 전송 오류: ${err.message}`);
        finishChunkedSend(btn, `오류: ${err.message}`);
    }
}

function finishChunkedSend(btn, errorMsg) {
    btn.disabled = false;
    btn.textContent = '전송 시작';
    btn.onclick = () => startSend();
    chunkedSendAbort = false;
    if (errorMsg) addLog('warn', errorMsg);
}

async function readFileChunk(file, seqNum, chunkSize) {
    const start = seqNum * chunkSize;
    const end = Math.min(start + chunkSize, file.size);
    const blob = file.slice(start, end);
    const arrayBuf = await blob.arrayBuffer();
    return new Uint8Array(arrayBuf);
}

function playSignalAsync(ctx, signal) {
    return new Promise((resolve) => {
        const sr = ctx.sampleRate || 44100;
        const buffer = ctx.createBuffer(1, signal.length, sr);
        buffer.getChannelData(0).set(signal);
        const source = ctx.createBufferSource();
        source.buffer = buffer;
        source.connect(ctx.destination);
        source.onended = resolve;
        source.start();
    });
}

function updateChunkProgressUI(sent, total, eta) {
    const panel = document.getElementById('chunk-progress');
    if (panel) {
        panel.style.display = 'block';
        const countEl = document.getElementById('chunk-count');
        const etaEl = document.getElementById('chunk-eta');
        if (countEl) countEl.textContent = `${sent} / ${total} 청크`;
        if (etaEl) etaEl.textContent = `남은 시간: ${formatETA(eta)}`;
    }
}

function formatETA(seconds) {
    if (!seconds || seconds <= 0 || !isFinite(seconds)) return '--';
    if (seconds < 60) return `${Math.round(seconds)}초`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}분 ${Math.round(seconds % 60)}초`;
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    return `${h}시간 ${m}분`;
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
    fullSignal = null;
    btn.textContent = '수신 중지';
    btn.classList.add('recording');
    showProgress();
    updateProgress(0, '수신 대기 중... (신호를 보내주세요)');
    addLog('info', '수신 대기 시작 — 상대방이 전송을 시작하세요');

    // 파형 트리머 숨기기 (이전 결과)
    document.getElementById('waveform-container').style.display = 'none';

    const ctx = getAudioContext();
    const source = ctx.createMediaStreamSource(micStream);

    // 실시간 레벨미터용 AnalyserNode
    levelAnalyser = ctx.createAnalyser();
    levelAnalyser.fftSize = 2048;
    source.connect(levelAnalyser);

    // 레벨미터 표시 시작
    document.getElementById('level-meter-container').style.display = 'block';
    const levelCanvas = document.getElementById('level-canvas');
    levelCanvas.width = levelCanvas.clientWidth * (window.devicePixelRatio || 1);
    drawLevelMeter(levelAnalyser, levelCanvas);

    // Use ScriptProcessorNode for broad compatibility
    const processor = ctx.createScriptProcessor(4096, 1, 1);
    let totalSamples = 0;
    const maxDuration = getMaxDuration();

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

    levelAnalyser.connect(processor);
    processor.connect(ctx.destination);

    // Store references for cleanup
    btn._source = source;
    btn._processor = processor;
}

function stopReceive() {
    isRecording = false;
    levelAnalyser = null;
    const btn = document.getElementById('btn-receive');
    btn.textContent = '수신 대기';
    btn.classList.remove('recording');

    // 레벨미터 숨기기
    document.getElementById('level-meter-container').style.display = 'none';

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
    fullSignal = new Float32Array(totalLen);
    let off = 0;
    for (const c of recordedChunks) { fullSignal.set(c, off); off += c.length; }
    recordedChunks = [];

    const duration = totalLen / 44100;
    addLog('info', `녹음 완료: ${duration.toFixed(1)}초 — 파형을 확인하고 구간을 선택하세요`);
    updateProgress(0, '트림 구간을 선택한 후 [선택 구간 복조]를 누르세요');

    // 파형 트리머 표시
    const waveformContainer = document.getElementById('waveform-container');
    waveformContainer.style.display = 'block';

    const waveformCanvas = document.getElementById('waveform-canvas');
    waveformCanvas.width = waveformCanvas.clientWidth * (window.devicePixelRatio || 1);
    waveformCanvas.height = 120 * (window.devicePixelRatio || 1);

    // 트림 슬라이더 초기화
    const trimStart = document.getElementById('trim-start');
    const trimEnd = document.getElementById('trim-end');
    trimStart.value = 0;
    trimEnd.value = 1000;

    updateTrimLabels();
    drawWaveform();

    // 슬라이더 이벤트
    trimStart.oninput = () => { updateTrimLabels(); drawWaveform(); };
    trimEnd.oninput = () => { updateTrimLabels(); drawWaveform(); };
}

function updateTrimLabels() {
    if (!fullSignal) return;
    const duration = fullSignal.length / 44100;
    const startVal = parseInt(document.getElementById('trim-start').value);
    const endVal = parseInt(document.getElementById('trim-end').value);
    const startSec = (startVal / 1000) * duration;
    const endSec = (endVal / 1000) * duration;
    const durSec = Math.max(0, endSec - startSec);

    document.getElementById('trim-start-label').textContent = `시작: ${startSec.toFixed(1)}s`;
    document.getElementById('trim-end-label').textContent = `종료: ${endSec.toFixed(1)}s`;
    document.getElementById('trim-duration-label').textContent = `구간: ${durSec.toFixed(1)}s`;
}

function demodulateTrimed() {
    if (!fullSignal) {
        addLog('warn', '녹음된 신호가 없습니다');
        return;
    }

    const startVal = parseInt(document.getElementById('trim-start').value);
    const endVal = parseInt(document.getElementById('trim-end').value);

    if (startVal >= endVal) {
        addLog('warn', '시작 지점이 종료 지점보다 앞에 있어야 합니다');
        return;
    }

    const trimStartSample = Math.floor((startVal / 1000) * fullSignal.length);
    const trimEndSample = Math.floor((endVal / 1000) * fullSignal.length);
    const signal = fullSignal.slice(trimStartSample, trimEndSample);

    const duration = signal.length / 44100;
    addLog('info', `트림된 구간 복조 시작: ${duration.toFixed(1)}초 (${formatSize(signal.length * 4)})`);
    updateProgress(0.3, '복조 중...');

    setTimeout(() => {
        try {
            const { config, modName, repetition } = getModemParams(modulation);
            setOFDMConfig(config);
            const result = decodeReceivedSignal(signal, modName, repetition);

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

// ============================================================
// Streaming Receiver — Real-time Frame Detection & Demodulation
// ============================================================

const RECV_STATE = { IDLE: 0, PREAMBLE_DETECTED: 1, COLLECTING_FRAME: 2, DEMODULATING: 3 };
let streamingReceiver = null;

class RingBuffer {
    constructor(capacity) {
        this.buffer = new Float32Array(capacity);
        this.capacity = capacity;
        this.writePos = 0;
        this.totalWritten = 0;
    }

    write(samples) {
        for (let i = 0; i < samples.length; i++) {
            this.buffer[this.writePos] = samples[i];
            this.writePos = (this.writePos + 1) % this.capacity;
        }
        this.totalWritten += samples.length;
    }

    // Get samples by global position (from totalWritten coordinate)
    getRange(globalStart, length) {
        const oldest = this.totalWritten - this.capacity;
        if (globalStart < oldest) return null; // data already overwritten
        const out = new Float32Array(length);
        const startInBuf = ((globalStart % this.capacity) + this.capacity) % this.capacity;
        for (let i = 0; i < length; i++) {
            out[i] = this.buffer[(startInBuf + i) % this.capacity];
        }
        return out;
    }

    // How many samples are available from a given global position
    availableFrom(globalStart) {
        return this.totalWritten - globalStart;
    }
}

class ChunkAssembler {
    constructor() {
        this.totalChunks = 0;
        this.totalFileSize = 0;
        this.chunkSize = 0;
        this.fileName = '';
        this.receivedBitmap = null;
        this.receivedCount = 0;
        this.crcErrors = 0;
        this.dbName = 'audioModemChunks';
        this.db = null;
    }

    async handleMetadataFrame(meta) {
        this.totalChunks = meta.totalChunks;
        this.totalFileSize = meta.totalFileSize;
        this.chunkSize = meta.chunkSize;
        this.fileName = meta.fileName;
        this.receivedBitmap = new Uint8Array(Math.ceil(this.totalChunks / 8));
        this.receivedCount = 0;
        this.crcErrors = 0;

        // Initialize IndexedDB
        if (this.db) this.db.close();
        await this._openDB();
        // Clear previous data
        const tx = this.db.transaction('chunks', 'readwrite');
        tx.objectStore('chunks').clear();
        await new Promise((resolve, reject) => { tx.oncomplete = resolve; tx.onerror = reject; });
    }

    async handleDataChunk(seqNum, data, crcValid) {
        if (!this.receivedBitmap) return;
        if (seqNum >= this.totalChunks) return;

        if (!crcValid) {
            this.crcErrors++;
            return;
        }

        // Mark as received
        const byteIdx = seqNum >> 3;
        const bitIdx = seqNum & 7;
        if (this.receivedBitmap[byteIdx] & (1 << bitIdx)) return; // duplicate
        this.receivedBitmap[byteIdx] |= (1 << bitIdx);
        this.receivedCount++;

        // Store in IndexedDB
        const tx = this.db.transaction('chunks', 'readwrite');
        tx.objectStore('chunks').put({ seqNum, data: new Uint8Array(data) });
        await new Promise((resolve, reject) => { tx.oncomplete = resolve; tx.onerror = reject; });
    }

    isReceived(seqNum) {
        if (!this.receivedBitmap) return false;
        return !!(this.receivedBitmap[seqNum >> 3] & (1 << (seqNum & 7)));
    }

    isComplete() {
        return this.receivedCount === this.totalChunks;
    }

    getMissingChunks() {
        const missing = [];
        for (let i = 0; i < this.totalChunks; i++) {
            if (!this.isReceived(i)) missing.push(i);
        }
        return missing;
    }

    async assembleFile() {
        const result = new Uint8Array(this.totalFileSize);
        const tx = this.db.transaction('chunks', 'readonly');
        const store = tx.objectStore('chunks');

        for (let i = 0; i < this.totalChunks; i++) {
            const req = store.get(i);
            const record = await new Promise((resolve, reject) => {
                req.onsuccess = () => resolve(req.result);
                req.onerror = reject;
            });
            if (record) {
                const offset = i * this.chunkSize;
                result.set(record.data, offset);
            }
        }

        return result.slice(0, this.totalFileSize);
    }

    async _openDB() {
        return new Promise((resolve, reject) => {
            const req = indexedDB.open(this.dbName, 1);
            req.onupgradeneeded = (e) => {
                const db = e.target.result;
                if (!db.objectStoreNames.contains('chunks')) {
                    db.createObjectStore('chunks', { keyPath: 'seqNum' });
                }
            };
            req.onsuccess = (e) => { this.db = e.target.result; resolve(); };
            req.onerror = (e) => reject(e);
        });
    }

    cleanup() {
        if (this.db) { this.db.close(); this.db = null; }
    }
}

class StreamingReceiver {
    constructor(modName, repetition) {
        this.modName = modName;
        this.repetition = repetition;

        // Ring buffer: enough for 2 max frames + some margin
        const maxPayload = 4096 + 16; // max chunk + overhead
        const maxFrameSamples = estimateFrameSamples(maxPayload, modName, repetition);
        const capacity = maxFrameSamples * 3 + 8192;
        this.ringBuffer = new RingBuffer(capacity);

        this.assembler = new ChunkAssembler();
        this.state = RECV_STATE.IDLE;
        this.metaReceived = false;

        // Auto-correlation state
        this.half = OFDM.FFT_SIZE / 2;
        this.acP = 0;
        this.acRa = 0;
        this.acRb = 0;
        this.acInitialized = false;
        this.acScanPos = 0; // global scan position

        // Preamble detection state
        this.preambleGlobalPos = -1;
        this.expectedFrameEnd = -1;

        // DC removal state (exponential moving average)
        this.dcAlpha = 0.999;
        this.dcMean = 0;

        // Stats
        this.framesDecoded = 0;
        this.frameErrors = 0;
        this.startTime = Date.now();

        // Pre-generate preamble for cross-correlation
        this.pre1 = generatePreambleSymbol1();
        this.pre1Energy = 0;
        for (let i = 0; i < this.pre1.length; i++) this.pre1Energy += this.pre1[i] * this.pre1[i];
    }

    // Called from ScriptProcessor callback
    processAudioBlock(inputSamples) {
        // DC removal via exponential moving average
        const cleaned = new Float32Array(inputSamples.length);
        for (let i = 0; i < inputSamples.length; i++) {
            this.dcMean = this.dcAlpha * this.dcMean + (1 - this.dcAlpha) * inputSamples[i];
            cleaned[i] = inputSamples[i] - this.dcMean;
        }

        this.ringBuffer.write(cleaned);

        switch (this.state) {
            case RECV_STATE.IDLE:
                this._scanForPreamble();
                break;
            case RECV_STATE.PREAMBLE_DETECTED:
                this._refineAndCollect();
                break;
            case RECV_STATE.COLLECTING_FRAME:
                this._checkFrameComplete();
                break;
            case RECV_STATE.DEMODULATING:
                // handled async
                break;
        }
    }

    _scanForPreamble() {
        const rb = this.ringBuffer;
        const half = this.half;
        const totalWritten = rb.totalWritten;
        const oldest = totalWritten - rb.capacity;

        // Ensure scan position is valid
        if (this.acScanPos < oldest + 2 * half) {
            this.acScanPos = Math.max(oldest + 2 * half, 0);
            this.acInitialized = false;
        }

        // Scan through new samples
        const scanEnd = totalWritten - 2 * half;
        if (this.acScanPos > scanEnd) return;

        if (!this.acInitialized) {
            // Initialize auto-correlation at acScanPos
            this.acP = 0; this.acRa = 0; this.acRb = 0;
            const seg = rb.getRange(this.acScanPos, 2 * half);
            if (!seg) return;
            for (let m = 0; m < half; m++) {
                const a = seg[m], b = seg[m + half];
                this.acP += a * b;
                this.acRa += a * a;
                this.acRb += b * b;
            }
            this.acInitialized = true;
        }

        const minEnergy = 0.001;
        let bestMetric = 0, bestPos = -1;

        while (this.acScanPos <= scanEnd) {
            if (this.acRa > minEnergy && this.acRb > minEnergy) {
                const metric = (this.acP * this.acP) / (this.acRa * this.acRb);
                if (metric > 0.5 && metric > bestMetric) {
                    bestMetric = metric;
                    bestPos = this.acScanPos;
                }
            }

            if (this.acScanPos < scanEnd) {
                // Sliding update
                const seg3 = rb.getRange(this.acScanPos, 2 * half + 1);
                if (!seg3) break;
                const aOut = seg3[0], mid = seg3[half], bIn = seg3[2 * half];
                this.acP  += mid * bIn  - aOut * mid;
                this.acRa += mid * mid  - aOut * aOut;
                this.acRb += bIn * bIn  - mid  * mid;
            }
            this.acScanPos++;

            // If we found a strong peak and metric is dropping, commit
            if (bestMetric > 0.5 && bestPos >= 0) {
                if (this.acRa > minEnergy && this.acRb > minEnergy) {
                    const currentMetric = (this.acP * this.acP) / (this.acRa * this.acRb);
                    if (currentMetric < bestMetric * 0.7) {
                        // Past the peak
                        this.preambleGlobalPos = bestPos;
                        this.state = RECV_STATE.PREAMBLE_DETECTED;
                        return;
                    }
                }
            }
        }

        // End of buffer — if we have a candidate, use it
        if (bestMetric > 0.5 && bestPos >= 0) {
            this.preambleGlobalPos = bestPos;
            this.state = RECV_STATE.PREAMBLE_DETECTED;
        }
    }

    _refineAndCollect() {
        const rb = this.ringBuffer;
        const pre1 = this.pre1;
        const pLen = pre1.length;
        const searchRadius = OFDM.CP_LEN * 3;

        // Need enough samples for cross-correlation
        const needed = this.preambleGlobalPos + pLen + searchRadius;
        if (rb.totalWritten < needed) return; // wait for more samples

        const fineStart = Math.max(rb.totalWritten - rb.capacity, this.preambleGlobalPos - searchRadius);
        const fineEnd = Math.min(rb.totalWritten - pLen, this.preambleGlobalPos + searchRadius);

        let bestMetric = -Infinity, bestPos = this.preambleGlobalPos;

        for (let d = fineStart; d <= fineEnd; d++) {
            const seg = rb.getRange(d, pLen);
            if (!seg) continue;
            let corr = 0, sEnergy = 0;
            for (let i = 0; i < pLen; i++) {
                corr += seg[i] * pre1[i];
                sEnergy += seg[i] * seg[i];
            }
            const denom = Math.sqrt(sEnergy * this.pre1Energy);
            if (denom > 0.001) {
                const metric = corr / denom;
                if (metric > bestMetric) { bestMetric = metric; bestPos = d; }
            }
        }

        if (bestMetric < 0.1) {
            // False positive — back to idle
            this.state = RECV_STATE.IDLE;
            this.acInitialized = false;
            return;
        }

        this.preambleGlobalPos = bestPos;

        // Estimate frame length: we need enough for preamble + CE + some data
        // We don't know payload size yet, so collect a generous amount
        // For metadata: ~16 bytes payload
        // For data: up to chunkSize + 11 bytes overhead
        const maxPayload = this.metaReceived
            ? (this.assembler.chunkSize || 4096) + 11
            : 280; // metadata is small
        const frameSamples = estimateFrameSamples(maxPayload, this.modName, this.repetition);
        this.expectedFrameEnd = this.preambleGlobalPos + frameSamples;
        this.state = RECV_STATE.COLLECTING_FRAME;
    }

    _checkFrameComplete() {
        if (this.ringBuffer.totalWritten < this.expectedFrameEnd) return;

        this.state = RECV_STATE.DEMODULATING;
        this._demodulateFrame();
    }

    async _demodulateFrame() {
        const rb = this.ringBuffer;
        const frameLen = this.expectedFrameEnd - this.preambleGlobalPos;
        let frameSamples = rb.getRange(this.preambleGlobalPos, frameLen);

        if (!frameSamples) {
            this.frameErrors++;
            this._resetToIdle();
            return;
        }

        // Normalize this frame segment independently
        let mx = 0;
        for (let i = 0; i < frameSamples.length; i++) mx = Math.max(mx, Math.abs(frameSamples[i]));
        if (mx > 1e-6) {
            const normSamples = new Float32Array(frameSamples.length);
            for (let i = 0; i < frameSamples.length; i++) normSamples[i] = frameSamples[i] / mx;
            frameSamples = normSamples;
        }

        try {
            const result = decodeChunkFrame(frameSamples, this.modName, this.repetition);

            if (result.error) {
                this.frameErrors++;
                addLog('warn', `프레임 복조 실패: ${result.error}`);
                this._resetToIdle();
                return;
            }

            this.framesDecoded++;

            if (result.frameType === FRAME_META) {
                if (result.crcValid) {
                    await this.assembler.handleMetadataFrame(result);
                    this.metaReceived = true;
                    addLog('success', `메타데이터 수신: ${result.fileName} (${formatSize(result.totalFileSize)}, ${result.totalChunks}개 청크)`);
                    updateStreamingUI(this);
                    const fnEl = document.getElementById('chunk-filename');
                    if (fnEl) fnEl.textContent = `파일: ${result.fileName} (${formatSize(result.totalFileSize)})`;
                } else {
                    this.frameErrors++;
                    addLog('error', '메타데이터 CRC 오류');
                }
            } else if (result.frameType === FRAME_DATA) {
                await this.assembler.handleDataChunk(result.seqNum, result.data, result.crcValid);
                if (result.crcValid) {
                    addLog('info', `청크 ${result.seqNum + 1}/${this.assembler.totalChunks} 수신 (${formatSize(result.dataLen)})`);
                } else {
                    addLog('warn', `청크 ${result.seqNum + 1} CRC 오류`);
                }
                updateStreamingUI(this);
                drawChunkBitmap(this.assembler);

                if (this.assembler.isComplete()) {
                    addLog('success', '모든 청크 수신 완료! 파일을 조립합니다...');
                    await this._assembleAndDownload();
                }
            }
        } catch (err) {
            this.frameErrors++;
            addLog('error', `프레임 처리 오류: ${err.message}`);
        }

        this._resetToIdle();
    }

    _resetToIdle() {
        // Resume scanning after current frame
        this.acScanPos = this.expectedFrameEnd || (this.preambleGlobalPos + OFDM.SYMBOL_LEN);
        this.acInitialized = false;
        this.preambleGlobalPos = -1;
        this.expectedFrameEnd = -1;
        this.state = RECV_STATE.IDLE;
    }

    async _assembleAndDownload() {
        try {
            const fileData = await this.assembler.assembleFile();
            const fileName = this.assembler.fileName || 'received_file';
            addLog('success', `파일 조립 완료: ${fileName} (${formatSize(fileData.length)})`);
            updateProgress(1.0, `수신 완료: ${fileName}`);
            offerDownload(fileData, fileName);
        } catch (err) {
            addLog('error', `파일 조립 오류: ${err.message}`);
        }
    }

    cleanup() {
        this.assembler.cleanup();
    }
}

function updateStreamingUI(receiver) {
    const asm = receiver.assembler;
    const panel = document.getElementById('chunk-progress');
    if (!panel) return;
    panel.style.display = 'block';

    const countEl = document.getElementById('chunk-count');
    const errEl = document.getElementById('chunk-errors');
    const etaEl = document.getElementById('chunk-eta');

    if (countEl) countEl.textContent = `${asm.receivedCount} / ${asm.totalChunks} 청크`;
    if (errEl) errEl.textContent = `오류: ${asm.crcErrors + receiver.frameErrors}`;

    // ETA estimation
    if (asm.receivedCount > 0) {
        const elapsed = (Date.now() - receiver.startTime) / 1000;
        const rate = asm.receivedCount / elapsed;
        const remaining = (asm.totalChunks - asm.receivedCount) / rate;
        if (etaEl) etaEl.textContent = `남은 시간: ${formatETA(remaining)}`;
    }

    const progress = asm.totalChunks > 0 ? asm.receivedCount / asm.totalChunks : 0;
    updateProgress(progress, `청크 ${asm.receivedCount}/${asm.totalChunks} 수신 · 오류: ${asm.crcErrors}`);
}

function drawChunkBitmap(assembler) {
    const canvas = document.getElementById('chunk-bitmap-canvas');
    if (!canvas || !assembler.receivedBitmap) return;

    const dpr = window.devicePixelRatio || 1;
    canvas.width = canvas.clientWidth * dpr;
    canvas.height = 40 * dpr;
    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);
    const w = canvas.clientWidth;
    const h = 40;

    ctx.fillStyle = '#0f0f23';
    ctx.fillRect(0, 0, w, h);

    const total = assembler.totalChunks;
    if (total === 0) return;

    const cellW = Math.max(1, w / total);
    for (let i = 0; i < total; i++) {
        const x = (i / total) * w;
        if (assembler.isReceived(i)) {
            ctx.fillStyle = '#00ff88'; // received
        } else {
            ctx.fillStyle = '#333';    // not yet
        }
        ctx.fillRect(x, 0, Math.ceil(cellW), h);
    }
}

// --- Streaming Receive Start/Stop ---

let isStreamingReceive = false;

async function startStreamingReceive() {
    const btn = document.getElementById('btn-receive');

    if (isStreamingReceive) {
        stopStreamingReceive();
        return;
    }

    try {
        micStream = await navigator.mediaDevices.getUserMedia({
            audio: {
                echoCancellation: false,
                noiseSuppression: false,
                autoGainControl: false,
                sampleRate: 44100,
            }
        });
        addLog('info', '마이크 권한 허용됨 (스트리밍 모드)');
    } catch (err) {
        addLog('error', `마이크 접근 실패: ${err.message}`);
        return;
    }

    isStreamingReceive = true;
    btn.textContent = '수신 중지';
    btn.classList.add('recording');
    showProgress();
    updateProgress(0, '스트리밍 수신 대기 중... (신호를 보내주세요)');

    const { config, modName, repetition } = getModemParams(modulation);
    setOFDMConfig(config);

    streamingReceiver = new StreamingReceiver(modName, repetition);

    const ctx = getAudioContext();
    const source = ctx.createMediaStreamSource(micStream);

    levelAnalyser = ctx.createAnalyser();
    levelAnalyser.fftSize = 2048;
    source.connect(levelAnalyser);

    document.getElementById('level-meter-container').style.display = 'block';
    const levelCanvas = document.getElementById('level-canvas');
    levelCanvas.width = levelCanvas.clientWidth * (window.devicePixelRatio || 1);
    // Use isRecording flag for level meter draw loop
    isRecording = true;
    drawLevelMeter(levelAnalyser, levelCanvas);

    const processor = ctx.createScriptProcessor(4096, 1, 1);
    processor.onaudioprocess = (e) => {
        if (!isStreamingReceive) return;
        const input = e.inputBuffer.getChannelData(0);
        streamingReceiver.processAudioBlock(input);
    };

    levelAnalyser.connect(processor);
    processor.connect(ctx.destination);

    btn._source = source;
    btn._processor = processor;

    // Show chunk progress panel
    const chunkPanel = document.getElementById('chunk-progress');
    if (chunkPanel) chunkPanel.style.display = 'block';

    addLog('info', '스트리밍 수신 시작 — 실시간 프레임 탐지 활성화');
}

function stopStreamingReceive() {
    isStreamingReceive = false;
    isRecording = false;
    levelAnalyser = null;

    const btn = document.getElementById('btn-receive');
    btn.textContent = '수신 대기';
    btn.classList.remove('recording');

    document.getElementById('level-meter-container').style.display = 'none';

    if (btn._processor) { btn._processor.disconnect(); btn._processor = null; }
    if (btn._source) { btn._source.disconnect(); btn._source = null; }
    if (micStream) { micStream.getTracks().forEach(t => t.stop()); micStream = null; }

    if (streamingReceiver) {
        const asm = streamingReceiver.assembler;
        if (asm.totalChunks > 0 && !asm.isComplete()) {
            const missing = asm.getMissingChunks();
            addLog('warn', `수신 중지: ${asm.receivedCount}/${asm.totalChunks} 청크 수신, ${missing.length}개 누락`);
            if (asm.receivedCount > 0) {
                addLog('info', '수신된 청크로 부분 파일을 조립합니다...');
                streamingReceiver._assembleAndDownload().then(() => {
                    streamingReceiver.cleanup();
                    streamingReceiver = null;
                });
                return;
            }
        } else if (asm.isComplete()) {
            addLog('success', '모든 청크 수신 완료');
        }
        streamingReceiver.cleanup();
        streamingReceiver = null;
    }
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

// --- 실시간 레벨미터 ---
function drawLevelMeter(analyser, canvas) {
    const ctx = canvas.getContext('2d');
    const bufLen = analyser.fftSize;
    const dataArray = new Uint8Array(bufLen);
    const w = canvas.width;
    const h = canvas.height;

    function draw() {
        if (!isRecording) return;
        requestAnimationFrame(draw);

        analyser.getByteTimeDomainData(dataArray);

        // 배경
        ctx.fillStyle = '#0f0f23';
        ctx.fillRect(0, 0, w, h);

        // RMS 계산
        let sumSq = 0;
        for (let i = 0; i < bufLen; i++) {
            const v = (dataArray[i] - 128) / 128;
            sumSq += v * v;
        }
        const rms = Math.sqrt(sumSq / bufLen);
        const clipping = rms > 0.9;

        // 오실로스코프 파형
        ctx.beginPath();
        ctx.strokeStyle = clipping ? '#ff4444' : '#00d4ff';
        ctx.lineWidth = 1.5;
        const sliceWidth = w / bufLen;
        let x = 0;
        for (let i = 0; i < bufLen; i++) {
            const v = dataArray[i] / 128.0;
            const y = (v * h) / 2;
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
            x += sliceWidth;
        }
        ctx.stroke();

        // RMS 레벨 바 (하단)
        const barH = 4;
        const barW = Math.min(rms * w * 1.5, w);
        ctx.fillStyle = clipping ? '#ff4444' : '#00ff88';
        ctx.fillRect(0, h - barH, barW, barH);
        ctx.fillStyle = '#2a2a4a';
        ctx.fillRect(barW, h - barH, w - barW, barH);
    }

    draw();
}

// --- 파형 그리기 (트리머) ---
function drawWaveform() {
    if (!fullSignal) return;
    const canvas = document.getElementById('waveform-canvas');
    const ctx = canvas.getContext('2d');
    const w = canvas.width;
    const h = canvas.height;
    const signal = fullSignal;
    const len = signal.length;

    // 트림 위치 계산
    const startVal = parseInt(document.getElementById('trim-start').value);
    const endVal = parseInt(document.getElementById('trim-end').value);
    const trimStartX = (startVal / 1000) * w;
    const trimEndX = (endVal / 1000) * w;

    // 배경
    ctx.fillStyle = '#0f0f23';
    ctx.fillRect(0, 0, w, h);

    // 파형 그리기 (각 픽셀 = N samples의 min/max)
    const samplesPerPixel = Math.max(1, Math.floor(len / w));
    ctx.beginPath();
    ctx.strokeStyle = '#00d4ff';
    ctx.lineWidth = 1;

    // min/max envelope
    const midY = h / 2;
    for (let px = 0; px < w; px++) {
        const startIdx = Math.floor((px / w) * len);
        const endIdx = Math.min(startIdx + samplesPerPixel, len);
        let min = 1, max = -1;
        for (let i = startIdx; i < endIdx; i++) {
            if (signal[i] < min) min = signal[i];
            if (signal[i] > max) max = signal[i];
        }
        const y1 = midY - max * midY;
        const y2 = midY - min * midY;
        ctx.moveTo(px, y1);
        ctx.lineTo(px, y2);
    }
    ctx.stroke();

    // 트림 영역 밖 반투명 오버레이
    ctx.fillStyle = 'rgba(0,0,0,0.5)';
    if (trimStartX > 0) ctx.fillRect(0, 0, trimStartX, h);
    if (trimEndX < w) ctx.fillRect(trimEndX, 0, w - trimEndX, h);

    // 트림 핸들 세로선
    ctx.strokeStyle = '#00ff88';
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(trimStartX, 0); ctx.lineTo(trimStartX, h);
    ctx.moveTo(trimEndX, 0); ctx.lineTo(trimEndX, h);
    ctx.stroke();
}

// ============================================================
// Pre-Test — Audio Path Diagnostics
// ============================================================

let testRunning = false;

function setTestButtonsDisabled(disabled) {
    document.querySelectorAll('.test-btn').forEach(btn => btn.disabled = disabled);
}

// --- Ensure AudioContext is running (critical for mobile) ---
async function ensureAudioContext() {
    const ctx = getAudioContext();
    if (ctx.state === 'suspended') {
        await ctx.resume();
    }
    return ctx;
}

// --- Output Test ---
async function runOutputTest() {
    if (testRunning) return;
    testRunning = true;
    setTestButtonsDisabled(true);
    hideTestResults();

    const { config, modName, repetition } = getModemParams(modulation);
    setOFDMConfig(config);

    addLog('info', '출력 테스트 시작 — 스윕 톤 재생');

    try {
        const ctx = await ensureAudioContext();
        const sr = ctx.sampleRate;

        // 1. Sweep tone (1kHz → 10kHz, 2s)
        const sweep = generateSweepTone(1000, 10000, 2.0, sr);
        await playSignalAsync(ctx, sweep);
        addLog('info', '스윕 톤 완료 — OFDM 테스트 심볼 재생');

        // 2. OFDM test symbol (preamble + CE)
        const { signal } = generateTestSignal(modName, repetition);
        await playSignalAsync(ctx, signal);

        addLog('success', '출력 테스트 완료 — 스피커/케이블에서 소리가 들렸다면 정상입니다');
        showTestResult('출력 테스트 완료', `스피커 또는 케이블에서 스윕 톤과 OFDM 신호가 정상적으로 재생되었습니다.\n(샘플레이트: ${sr} Hz)`, 'good');
    } catch (err) {
        addLog('error', `출력 테스트 오류: ${err.message}`);
    }

    testRunning = false;
    setTestButtonsDisabled(false);
}

// --- Input Test ---
async function runInputTest() {
    if (testRunning) return;
    testRunning = true;
    setTestButtonsDisabled(true);
    hideTestResults();

    addLog('info', '입력 테스트 시작 — 3초간 녹음');

    let stream;
    try {
        stream = await navigator.mediaDevices.getUserMedia({
            audio: {
                echoCancellation: false,
                noiseSuppression: false,
                autoGainControl: false,
            }
        });
    } catch (err) {
        addLog('error', `마이크 접근 실패: ${err.message}`);
        testRunning = false;
        setTestButtonsDisabled(false);
        return;
    }

    try {
        const ctx = await ensureAudioContext();
        const sr = ctx.sampleRate;
        const source = ctx.createMediaStreamSource(stream);
        const processor = ctx.createScriptProcessor(4096, 1, 1);
        const chunks = [];
        let totalSamples = 0;
        const maxSamples = 3 * sr;

        await new Promise((resolve) => {
            // Timeout safety: resolve after 5s even if not enough samples
            const timeout = setTimeout(() => {
                addLog('warn', '녹음 타임아웃 — 수집된 데이터로 분석합니다');
                resolve();
            }, 5000);

            processor.onaudioprocess = (e) => {
                const input = e.inputBuffer.getChannelData(0);
                chunks.push(new Float32Array(input));
                totalSamples += input.length;
                if (totalSamples >= maxSamples) {
                    clearTimeout(timeout);
                    resolve();
                }
            };
            source.connect(processor);
            processor.connect(ctx.destination);
        });

        processor.disconnect();
        source.disconnect();
        stream.getTracks().forEach(t => t.stop());
        stream = null;

        if (totalSamples === 0) {
            addLog('error', '녹음된 데이터가 없습니다. 마이크 권한을 확인하세요.');
            testRunning = false;
            setTestButtonsDisabled(false);
            return;
        }

        // Concatenate
        const recorded = new Float32Array(totalSamples);
        let off = 0;
        for (const c of chunks) { recorded.set(c, off); off += c.length; }

        // Analyze
        let sumSq = 0, peak = 0;
        for (let i = 0; i < recorded.length; i++) {
            const v = Math.abs(recorded[i]);
            sumSq += recorded[i] * recorded[i];
            if (v > peak) peak = v;
        }
        const rms = Math.sqrt(sumSq / recorded.length);
        const rmsDb = rms > 0 ? 20 * Math.log10(rms) : -Infinity;
        const peakDb = peak > 0 ? 20 * Math.log10(peak) : -Infinity;

        // Noise floor: average RMS of bottom 10% blocks
        const blockSize = 1024;
        const numBlocks = Math.floor(recorded.length / blockSize);
        const blockRms = [];
        for (let b = 0; b < numBlocks; b++) {
            let bSum = 0;
            for (let i = 0; i < blockSize; i++) {
                const v = recorded[b * blockSize + i];
                bSum += v * v;
            }
            blockRms.push(Math.sqrt(bSum / blockSize));
        }
        blockRms.sort((a, b) => a - b);
        const bottom10 = blockRms.slice(0, Math.max(1, Math.floor(numBlocks * 0.1)));
        const noiseFloor = bottom10.reduce((s, v) => s + v, 0) / bottom10.length;
        const noiseDb = noiseFloor > 0 ? 20 * Math.log10(noiseFloor) : -Infinity;

        // FFT spectrum
        const fftSize = 2048;
        const specLen = Math.min(recorded.length, fftSize);
        const fftRe = new Float64Array(fftSize);
        const fftIm = new Float64Array(fftSize);
        const midStart = Math.max(0, Math.floor((recorded.length - fftSize) / 2));
        for (let i = 0; i < specLen; i++) fftRe[i] = recorded[midStart + i];
        const [specRe, specIm] = fft(fftRe, fftIm);

        const magnitudes = new Float32Array(fftSize / 2);
        for (let i = 0; i < fftSize / 2; i++) {
            magnitudes[i] = Math.sqrt(specRe[i] * specRe[i] + specIm[i] * specIm[i]);
        }

        // Draw spectrum
        const canvas = document.getElementById('test-spectrum-canvas');
        canvas.style.display = 'block';
        drawSpectrum(canvas, magnitudes);

        // Assessment
        const clipping = peak > 0.95;
        const lowLevel = rms < 0.005;
        let quality, message;
        if (clipping) {
            quality = 'poor';
            message = `클리핑 감지! 볼륨을 낮춰주세요.\nRMS: ${rmsDb.toFixed(1)} dB · 피크: ${peakDb.toFixed(1)} dB · 노이즈: ${noiseDb.toFixed(1)} dB`;
        } else if (lowLevel) {
            quality = 'poor';
            message = `입력 레벨이 너무 낮습니다. 볼륨을 높이거나 마이크를 확인하세요.\nRMS: ${rmsDb.toFixed(1)} dB · 피크: ${peakDb.toFixed(1)} dB · 노이즈: ${noiseDb.toFixed(1)} dB`;
        } else {
            quality = rms > 0.02 ? 'excellent' : 'good';
            message = `입력 감도 양호\nRMS: ${rmsDb.toFixed(1)} dB · 피크: ${peakDb.toFixed(1)} dB · 노이즈: ${noiseDb.toFixed(1)} dB`;
        }
        message += `\n(샘플레이트: ${sr} Hz, 녹음: ${(totalSamples / sr).toFixed(1)}초)`;

        addLog(quality === 'poor' ? 'warn' : 'success', `입력 테스트: RMS=${rmsDb.toFixed(1)}dB, 피크=${peakDb.toFixed(1)}dB`);
        showTestResult('입력 테스트 결과', message, quality);
    } catch (err) {
        addLog('error', `입력 테스트 오류: ${err.message}`);
    } finally {
        if (stream) stream.getTracks().forEach(t => t.stop());
    }

    testRunning = false;
    setTestButtonsDisabled(false);
}

// --- Loopback Test ---
async function runLoopbackTest() {
    if (testRunning) return;
    testRunning = true;
    setTestButtonsDisabled(true);
    hideTestResults();

    const { config, modName, repetition } = getModemParams(modulation);
    setOFDMConfig(config);

    addLog('info', '루프백 테스트 시작 — 테스트 신호 재생 + 동시 녹음');

    let stream;
    try {
        stream = await navigator.mediaDevices.getUserMedia({
            audio: {
                echoCancellation: false,
                noiseSuppression: false,
                autoGainControl: false,
            }
        });
    } catch (err) {
        addLog('error', `마이크 접근 실패: ${err.message}`);
        testRunning = false;
        setTestButtonsDisabled(false);
        return;
    }

    try {
        const ctx = await ensureAudioContext();
        const sr = ctx.sampleRate;
        const source = ctx.createMediaStreamSource(stream);
        const processor = ctx.createScriptProcessor(4096, 1, 1);
        const chunks = [];
        let totalSamples = 0;
        let recording = true;

        processor.onaudioprocess = (e) => {
            if (!recording) return;
            const input = e.inputBuffer.getChannelData(0);
            chunks.push(new Float32Array(input));
            totalSamples += input.length;
        };
        source.connect(processor);
        processor.connect(ctx.destination);

        // Wait briefly to ensure processor is running before playback
        await sleep(200);

        // Generate and play test signal
        const { signal: testSignal, testData } = generateTestSignal(modName, repetition);
        await playSignalAsync(ctx, testSignal);

        // Wait 1 second after playback
        await sleep(1000);

        // Stop recording
        recording = false;
        processor.disconnect();
        source.disconnect();
        stream.getTracks().forEach(t => t.stop());
        stream = null;

        if (totalSamples === 0) {
            addLog('error', '녹음된 데이터가 없습니다. 마이크를 확인하세요.');
            testRunning = false;
            setTestButtonsDisabled(false);
            return;
        }

        // Concatenate
        const recorded = new Float32Array(totalSamples);
        let off = 0;
        for (const c of chunks) { recorded.set(c, off); off += c.length; }

        addLog('info', `녹음 완료: ${(totalSamples / sr).toFixed(1)}초 — 분석 중...`);

        // Analyze
        const result = analyzeLoopback(recorded, modName, repetition, testData);

        // Draw channel response if available
        if (result.channelMagnitude.length > 0) {
            const canvas = document.getElementById('test-channel-canvas');
            canvas.style.display = 'block';
            drawChannelResponse(canvas, result.channelMagnitude);
        }

        // Build result message
        const corrPct = (result.correlation * 100).toFixed(1);
        const berPct = (result.ber * 100).toFixed(1);
        let recommendedMod = '';
        if (result.quality === 'excellent') {
            recommendedMod = '16-QAM 또는 QPSK 사용 가능';
        } else if (result.quality === 'good') {
            recommendedMod = 'QPSK 또는 BPSK-ACOUSTIC 권장';
        } else {
            recommendedMod = 'BPSK-반복 또는 협대역 권장';
        }

        const message = [
            `프리앰블 탐지: ${result.detected ? '성공' : '실패'}`,
            `상관 피크: ${corrPct}%`,
            `BER: ${berPct}%`,
            `SNR 추정: ${isFinite(result.snrEstimate) ? result.snrEstimate.toFixed(1) + ' dB' : 'N/A'}`,
            `권장 변조: ${recommendedMod}`,
            `(샘플레이트: ${sr} Hz)`,
        ].join('\n');

        addLog(result.quality === 'poor' ? 'warn' : 'success',
            `루프백 테스트: 상관=${corrPct}%, BER=${berPct}%, 품질=${result.quality}`);
        showTestResult('루프백 테스트 결과', message, result.quality);
    } catch (err) {
        addLog('error', `루프백 테스트 오류: ${err.message}`);
    } finally {
        if (stream) stream.getTracks().forEach(t => t.stop());
    }

    testRunning = false;
    setTestButtonsDisabled(false);
}

// --- Visualization Helpers ---

function drawSpectrum(canvas, magnitudes) {
    const dpr = window.devicePixelRatio || 1;
    canvas.width = canvas.clientWidth * dpr;
    canvas.height = 100 * dpr;
    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);
    const w = canvas.clientWidth;
    const h = 100;

    ctx.fillStyle = '#0f0f23';
    ctx.fillRect(0, 0, w, h);

    // Convert to dB
    const dbValues = new Float32Array(magnitudes.length);
    let maxDb = -Infinity;
    for (let i = 0; i < magnitudes.length; i++) {
        dbValues[i] = magnitudes[i] > 1e-10 ? 20 * Math.log10(magnitudes[i]) : -100;
        if (dbValues[i] > maxDb) maxDb = dbValues[i];
    }
    const minDb = maxDb - 80;

    // OFDM band highlight
    const freqPerBin = 44100 / (magnitudes.length * 2);
    const bandStartHz = OFDM.SUB_START * freqPerBin;
    const bandEndHz = OFDM.SUB_END * freqPerBin;
    const xBandStart = (bandStartHz / 22050) * w;
    const xBandEnd = (bandEndHz / 22050) * w;
    ctx.fillStyle = 'rgba(0,212,255,0.08)';
    ctx.fillRect(xBandStart, 0, xBandEnd - xBandStart, h);

    // Spectrum line
    ctx.beginPath();
    ctx.strokeStyle = '#00d4ff';
    ctx.lineWidth = 1;
    for (let i = 0; i < magnitudes.length; i++) {
        const x = (i / magnitudes.length) * w;
        const normalized = (dbValues[i] - minDb) / (maxDb - minDb);
        const y = h - Math.max(0, Math.min(1, normalized)) * (h - 4);
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
    }
    ctx.stroke();

    // Axis labels
    ctx.fillStyle = '#666';
    ctx.font = '10px monospace';
    ctx.fillText('0 kHz', 2, h - 2);
    ctx.fillText('11 kHz', w / 2 - 20, h - 2);
    ctx.fillText('22 kHz', w - 36, h - 2);
}

function drawChannelResponse(canvas, channelMag) {
    const dpr = window.devicePixelRatio || 1;
    canvas.width = canvas.clientWidth * dpr;
    canvas.height = 100 * dpr;
    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);
    const w = canvas.clientWidth;
    const h = 100;

    ctx.fillStyle = '#0f0f23';
    ctx.fillRect(0, 0, w, h);

    if (channelMag.length === 0) return;

    // Convert to dB
    const dbValues = channelMag.map(m => m > 1e-10 ? 20 * Math.log10(m) : -60);
    let maxDb = -Infinity, minDb = Infinity;
    for (const d of dbValues) {
        if (d > maxDb) maxDb = d;
        if (d < minDb) minDb = d;
    }
    const range = Math.max(maxDb - minDb, 1);
    const threshold = maxDb - 20; // 20dB below peak = severe attenuation

    // Draw bars
    const barW = Math.max(1, w / channelMag.length);
    for (let i = 0; i < channelMag.length; i++) {
        const x = (i / channelMag.length) * w;
        const normalized = (dbValues[i] - minDb) / range;
        const barH = Math.max(1, normalized * (h - 10));

        ctx.fillStyle = dbValues[i] < threshold ? '#ff4444' : '#00d4ff';
        ctx.fillRect(x, h - barH, Math.ceil(barW), barH);
    }

    // Labels
    ctx.fillStyle = '#666';
    ctx.font = '10px monospace';
    ctx.fillText(`채널 응답 (${channelMag.length} 서브캐리어)`, 4, 12);
    ctx.fillText(`${maxDb.toFixed(0)} dB`, w - 40, 12);
}

function getQualityBadge(quality) {
    const map = {
        excellent: '<span class="quality-badge quality-excellent">Excellent</span>',
        good: '<span class="quality-badge quality-good">Good</span>',
        poor: '<span class="quality-badge quality-poor">Poor</span>',
    };
    return map[quality] || map.poor;
}

function showTestResult(title, message, quality) {
    const el = document.getElementById('test-result');
    el.style.display = 'block';
    el.innerHTML = `
        <div class="test-result-box">
            <div style="display:flex; align-items:center; gap:8px; margin-bottom:8px">
                <strong style="font-size:0.9rem">${title}</strong>
                ${getQualityBadge(quality)}
            </div>
            <pre style="color:#aaa; font-size:0.8rem; font-family:inherit; white-space:pre-wrap; margin:0">${message}</pre>
        </div>`;
}

function hideTestResults() {
    document.getElementById('test-result').style.display = 'none';
    document.getElementById('test-spectrum-canvas').style.display = 'none';
    document.getElementById('test-channel-canvas').style.display = 'none';
}
