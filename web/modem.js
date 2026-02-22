// ============================================================
// OFDM Audio Modem — Pure JavaScript Implementation
// ============================================================

// --- FFT ---
function fft(re, im) {
    const n = re.length;
    const outRe = Float64Array.from(re);
    const outIm = Float64Array.from(im);
    bitReverse(outRe, outIm);
    fftIterative(outRe, outIm, false);
    return [outRe, outIm];
}

function ifft(re, im) {
    const n = re.length;
    const outRe = Float64Array.from(re);
    const outIm = Float64Array.from(im);
    bitReverse(outRe, outIm);
    fftIterative(outRe, outIm, true);
    const scale = 1 / n;
    for (let i = 0; i < n; i++) { outRe[i] *= scale; outIm[i] *= scale; }
    return [outRe, outIm];
}

function fftIterative(re, im, inverse) {
    const n = re.length;
    for (let size = 2; size <= n; size <<= 1) {
        const half = size >> 1;
        const sign = inverse ? 1 : -1;
        const angle = sign * 2 * Math.PI / size;
        const wnRe = Math.cos(angle), wnIm = Math.sin(angle);
        for (let start = 0; start < n; start += size) {
            let wRe = 1, wIm = 0;
            for (let j = 0; j < half; j++) {
                const i1 = start + j, i2 = start + j + half;
                const tRe = wRe * re[i2] - wIm * im[i2];
                const tIm = wRe * im[i2] + wIm * re[i2];
                re[i2] = re[i1] - tRe; im[i2] = im[i1] - tIm;
                re[i1] += tRe; im[i1] += tIm;
                const nwRe = wRe * wnRe - wIm * wnIm;
                wIm = wRe * wnIm + wIm * wnRe;
                wRe = nwRe;
            }
        }
    }
}

function bitReverse(re, im) {
    const n = re.length;
    let bits = 0, tmp = n;
    while (tmp > 1) { bits++; tmp >>= 1; }
    for (let i = 0; i < n; i++) {
        const j = revBits(i, bits);
        if (i < j) {
            let t = re[i]; re[i] = re[j]; re[j] = t;
            t = im[i]; im[i] = im[j]; im[j] = t;
        }
    }
}

function revBits(x, bits) {
    let r = 0;
    for (let i = 0; i < bits; i++) { r = (r << 1) | (x & 1); x >>= 1; }
    return r;
}

// --- OFDM Parameters ---
const OFDM_CONFIGS = {
    standard: {
        FFT_SIZE: 512, CP_LEN: 64, SYMBOL_LEN: 576, SAMPLE_RATE: 44100,
        SUB_START: 12, SUB_END: 232,
        PILOTS: [15, 29, 43, 57, 71, 85, 99, 113, 127, 141, 155, 169, 183, 197, 211, 225],
    },
    acoustic: {
        FFT_SIZE: 512, CP_LEN: 128, SYMBOL_LEN: 640, SAMPLE_RATE: 44100,
        SUB_START: 23, SUB_END: 93,   // ~2000Hz–8000Hz (스피커/마이크 안정 대역)
        PILOTS: [25, 35, 45, 55, 65, 75, 85],
    },
    narrowband: {
        FFT_SIZE: 512, CP_LEN: 256, SYMBOL_LEN: 768, SAMPLE_RATE: 44100,
        SUB_START: 35, SUB_END: 58,   // ~3000Hz–5000Hz (가장 안정적인 대역)
        PILOTS: [37, 45, 53],
    },
};

const OFDM = { ...OFDM_CONFIGS.standard };
OFDM.isPilot = (k) => OFDM.PILOTS.includes(k);
OFDM.numDataSubs = () => {
    let c = 0;
    for (let k = OFDM.SUB_START; k <= OFDM.SUB_END; k++) if (!OFDM.isPilot(k)) c++;
    return c;
};

function setOFDMConfig(name) {
    const cfg = OFDM_CONFIGS[name] || OFDM_CONFIGS.standard;
    Object.keys(cfg).forEach(k => { OFDM[k] = cfg[k]; });
}

// --- Constellation ---
const Constellations = {
    BPSK: { bps: 1, points: null },
    QPSK: { bps: 2, points: null },
    QAM16: { bps: 4, points: null },
};

function initConstellation(name) {
    const c = Constellations[name];
    if (c.points) return c;
    if (name === 'BPSK') {
        c.points = [[1, 0], [-1, 0]];
    } else if (name === 'QPSK') {
        const s = 1 / Math.SQRT2;
        c.points = [
            [s, s], [-s, s], [-s, -s], [s, -s]
        ];
    } else if (name === 'QAM16') {
        const raw = [];
        for (let i = 0; i < 16; i++) {
            const row = i >> 2, col = i & 3;
            const gr = row ^ (row >> 1), gc = col ^ (col >> 1);
            raw.push([2 * gc - 3, 2 * gr - 3]);
        }
        let avg = 0;
        for (const p of raw) avg += p[0] * p[0] + p[1] * p[1];
        avg /= raw.length;
        const s = 1 / Math.sqrt(avg);
        c.points = raw.map(p => [p[0] * s, p[1] * s]);
    }
    return c;
}

function constellationMap(c, bits) {
    let idx = 0;
    for (const b of bits) idx = (idx << 1) | (b & 1);
    const p = c.points[idx % c.points.length];
    return p;
}

function constellationDemap(c, re, im) {
    let minD = Infinity, minIdx = 0;
    for (let i = 0; i < c.points.length; i++) {
        const dr = re - c.points[i][0], di = im - c.points[i][1];
        const d = dr * dr + di * di;
        if (d < minD) { minD = d; minIdx = i; }
    }
    const bits = [];
    for (let i = c.bps - 1; i >= 0; i--) bits.push((minIdx >> i) & 1);
    return bits;
}

// --- Preamble (Schmidl-Cox) ---
function seededRandom(seed) {
    let s = seed;
    return () => { s = (s * 1103515245 + 12345) & 0x7fffffff; return s / 0x7fffffff; };
}

function generatePreambleSymbol1() {
    const re = new Float64Array(OFDM.FFT_SIZE);
    const im = new Float64Array(OFDM.FFT_SIZE);
    const rng = seededRandom(42);
    for (let k = OFDM.SUB_START; k <= OFDM.SUB_END; k += 2) {
        re[k] = rng() > 0.5 ? 1 : -1;
    }
    const n = OFDM.FFT_SIZE;
    for (let k = 1; k < n / 2; k++) { re[n - k] = re[k]; im[n - k] = -im[k]; }
    re[0] = 0; re[n / 2] = 0; im[n / 2] = 0;
    const [td] = ifft(re, im);
    return addCP(td);
}

function generatePreambleSymbol2() {
    const re = new Float64Array(OFDM.FFT_SIZE);
    const im = new Float64Array(OFDM.FFT_SIZE);
    const rng = seededRandom(43);
    for (let k = OFDM.SUB_START; k <= OFDM.SUB_END; k++) {
        re[k] = rng() > 0.5 ? 1 : -1;
    }
    const n = OFDM.FFT_SIZE;
    for (let k = 1; k < n / 2; k++) { re[n - k] = re[k]; im[n - k] = -im[k]; }
    re[0] = 0; re[n / 2] = 0; im[n / 2] = 0;
    const [td] = ifft(re, im);
    return addCP(td);
}

function generateChannelEstSymbol() {
    const re = new Float64Array(OFDM.FFT_SIZE);
    const im = new Float64Array(OFDM.FFT_SIZE);
    const knownRe = new Float64Array(OFDM.FFT_SIZE);
    const rng = seededRandom(44);
    for (let k = OFDM.SUB_START; k <= OFDM.SUB_END; k++) {
        const v = rng() > 0.5 ? 1 : -1;
        re[k] = v; knownRe[k] = v;
    }
    const n = OFDM.FFT_SIZE;
    for (let k = 1; k < n / 2; k++) { re[n - k] = re[k]; im[n - k] = -im[k]; }
    re[0] = 0; re[n / 2] = 0; im[n / 2] = 0;
    const [td] = ifft(re, im);
    return { samples: addCP(td), knownRe, knownIm: new Float64Array(OFDM.FFT_SIZE) };
}

function addCP(td) {
    const n = td.length, cp = OFDM.CP_LEN;
    const out = new Float32Array(cp + n);
    for (let i = 0; i < cp; i++) out[i] = td[n - cp + i];
    for (let i = 0; i < n; i++) out[cp + i] = td[i];
    return out;
}

// --- Signal Preprocessing (DC removal + normalize only) ---
// Note: bandpass filtering omitted because it distorts cross-correlation.
// The OFDM channel equalizer handles frequency response naturally.
function preprocessSignal(signal) {
    // 1. DC removal
    let mean = 0;
    for (let i = 0; i < signal.length; i++) mean += signal[i];
    mean /= signal.length;

    const out = new Float32Array(signal.length);
    let mx = 0;
    for (let i = 0; i < signal.length; i++) {
        out[i] = signal[i] - mean;
        mx = Math.max(mx, Math.abs(out[i]));
    }

    // 2. Normalize to unit peak
    if (mx > 1e-6) {
        for (let i = 0; i < out.length; i++) out[i] /= mx;
    }

    return out;
}

// --- Preamble Detection: Cross-Correlation (robust for acoustic) ---
function detectPreambleCrossCorr(signal) {
    const pre1 = generatePreambleSymbol1();
    const pLen = pre1.length;
    if (signal.length < pLen) return -1;

    // Template energy
    let tEnergy = 0;
    for (let i = 0; i < pLen; i++) tEnergy += pre1[i] * pre1[i];
    if (tEnergy < 1e-10) return -1;

    const end = signal.length - pLen;
    const step = Math.max(1, Math.floor(pLen / 10));

    // Coarse search
    let best = 0, bestIdx = -1;
    for (let d = 0; d <= end; d += step) {
        let corr = 0, sEnergy = 0;
        for (let i = 0; i < pLen; i++) {
            corr += signal[d + i] * pre1[i];
            sEnergy += signal[d + i] * signal[d + i];
        }
        const denom = Math.sqrt(sEnergy * tEnergy);
        if (denom > 0.001) {
            const metric = corr / denom;
            if (metric > best) { best = metric; bestIdx = d; }
        }
    }

    if (bestIdx < 0 || best < 0.15) return -1;

    // Fine search around coarse peak
    const fineStart = Math.max(0, bestIdx - step);
    const fineEnd = Math.min(end, bestIdx + step);
    best = 0;
    for (let d = fineStart; d <= fineEnd; d++) {
        let corr = 0, sEnergy = 0;
        for (let i = 0; i < pLen; i++) {
            corr += signal[d + i] * pre1[i];
            sEnergy += signal[d + i] * signal[d + i];
        }
        const denom = Math.sqrt(sEnergy * tEnergy);
        if (denom > 0.001) {
            const metric = corr / denom;
            if (metric > best) { best = metric; bestIdx = d; }
        }
    }

    return best > 0.15 ? bestIdx : -1;
}

// --- Preamble Detection: Auto-Correlation (Sliding Window, O(n)) ---
function detectPreamble(signal) {
    const half = OFDM.FFT_SIZE / 2; // 256
    const len = signal.length;
    if (len < 2 * half) return -1;

    // Compute initial P(0), Ra(0), Rb(0)
    let p = 0, ra = 0, rb = 0;
    for (let m = 0; m < half; m++) {
        const a = signal[m], b = signal[m + half];
        p += a * b;
        ra += a * a;
        rb += b * b;
    }

    let best = 0, bestIdx = -1;
    const end = len - 2 * half;
    const minEnergy = 0.01;

    for (let d = 0; d <= end; d++) {
        // Normalized metric: p² / (ra * rb) ∈ [0, 1] (Pearson r²)
        if (ra > minEnergy && rb > minEnergy) {
            const metric = (p * p) / (ra * rb);
            if (metric > best) { best = metric; bestIdx = d; }
        }
        if (d < end) {
            const aOut = signal[d], mid = signal[d + half], bIn = signal[d + 2 * half];
            p  += mid * bIn  - aOut * mid;
            ra += mid * mid  - aOut * aOut;
            rb += bIn * bIn  - mid  * mid;
        }
    }

    return best > 0.5 ? bestIdx : -1;
}

// --- Modulation ---
function modulateOFDM(bits, modName) {
    const c = initConstellation(modName);
    const bps = c.bps;
    const numDataSubs = OFDM.numDataSubs();
    const bitsPerSymbol = numDataSubs * bps;

    // Pad bits
    while (bits.length % bitsPerSymbol !== 0) bits.push(0);

    const numSymbols = bits.length / bitsPerSymbol;
    const allSamples = [];

    for (let s = 0; s < numSymbols; s++) {
        const symBits = bits.slice(s * bitsPerSymbol, (s + 1) * bitsPerSymbol);
        const specRe = new Float64Array(OFDM.FFT_SIZE);
        const specIm = new Float64Array(OFDM.FFT_SIZE);

        let di = 0;
        for (let k = OFDM.SUB_START; k <= OFDM.SUB_END; k++) {
            if (OFDM.isPilot(k)) {
                specRe[k] = 1; specIm[k] = 0;
            } else {
                const b = symBits.slice(di * bps, (di + 1) * bps);
                const p = constellationMap(c, b);
                specRe[k] = p[0]; specIm[k] = p[1];
                di++;
            }
        }

        // Hermitian symmetry
        const n = OFDM.FFT_SIZE;
        for (let k = 1; k < n / 2; k++) { specRe[n - k] = specRe[k]; specIm[n - k] = -specIm[k]; }
        specRe[0] = 0; specIm[0] = 0; specIm[n / 2] = 0;

        const [td] = ifft(specRe, specIm);
        const sym = addCP(td);
        allSamples.push(sym);
    }

    return { samples: allSamples, numSymbols, bitsPerSymbol };
}

// --- Demodulation ---
function demodulateOFDM(signal, modName, channelRe, channelIm) {
    const c = initConstellation(modName);
    const bps = c.bps;
    const numSymbols = Math.floor(signal.length / OFDM.SYMBOL_LEN);
    const allBits = [];

    for (let s = 0; s < numSymbols; s++) {
        const offset = s * OFDM.SYMBOL_LEN;
        // Remove CP
        const re = new Float64Array(OFDM.FFT_SIZE);
        const im = new Float64Array(OFDM.FFT_SIZE);
        for (let i = 0; i < OFDM.FFT_SIZE; i++) {
            re[i] = signal[offset + OFDM.CP_LEN + i] || 0;
        }

        // FFT
        const [specRe, specIm] = fft(re, im);

        // Equalize
        const eqRe = new Float64Array(OFDM.FFT_SIZE);
        const eqIm = new Float64Array(OFDM.FFT_SIZE);
        for (let k = OFDM.SUB_START; k <= OFDM.SUB_END; k++) {
            const hr = channelRe[k], hi = channelIm[k];
            const hMag = hr * hr + hi * hi;
            if (hMag > 1e-10) {
                eqRe[k] = (specRe[k] * hr + specIm[k] * hi) / hMag;
                eqIm[k] = (specIm[k] * hr - specRe[k] * hi) / hMag;
            } else {
                eqRe[k] = specRe[k]; eqIm[k] = specIm[k];
            }
        }

        // Phase correction from pilots
        let phaseSum = 0, pc = 0;
        for (const p of OFDM.PILOTS) {
            if (p >= OFDM.SUB_START && p <= OFDM.SUB_END && Math.abs(eqRe[p]) > 1e-6) {
                phaseSum += eqIm[p] / eqRe[p];
                pc++;
            }
        }
        const phase = pc > 0 ? phaseSum / pc : 0;

        // Demap
        for (let k = OFDM.SUB_START; k <= OFDM.SUB_END; k++) {
            if (!OFDM.isPilot(k)) {
                const cr = eqRe[k] + eqIm[k] * phase;
                const ci = eqIm[k] - eqRe[k] * phase;
                allBits.push(...constellationDemap(c, cr, ci));
            }
        }
    }

    return allBits;
}

// --- Channel Estimation ---
function estimateChannel(receivedSamples, knownRe, knownIm) {
    const re = new Float64Array(OFDM.FFT_SIZE);
    const im = new Float64Array(OFDM.FFT_SIZE);
    for (let i = 0; i < OFDM.FFT_SIZE; i++) {
        re[i] = receivedSamples[OFDM.CP_LEN + i] || 0;
    }
    const [specRe, specIm] = fft(re, im);

    const chRe = new Float64Array(OFDM.FFT_SIZE);
    const chIm = new Float64Array(OFDM.FFT_SIZE);
    for (let k = OFDM.SUB_START; k <= OFDM.SUB_END; k++) {
        const xr = knownRe[k], xi = knownIm[k];
        const d = xr * xr + xi * xi;
        if (d > 1e-10) {
            chRe[k] = (specRe[k] * xr + specIm[k] * xi) / d;
            chIm[k] = (specIm[k] * xr - specRe[k] * xi) / d;
        }
    }
    return [chRe, chIm];
}

// --- CRC-32 ---
const CRC32_TABLE = (() => {
    const t = new Uint32Array(256);
    for (let i = 0; i < 256; i++) {
        let c = i;
        for (let j = 0; j < 8; j++) c = (c & 1) ? (0xEDB88320 ^ (c >>> 1)) : (c >>> 1);
        t[i] = c;
    }
    return t;
})();

function crc32(data) {
    let c = 0xFFFFFFFF;
    for (const b of data) c = CRC32_TABLE[(c ^ b) & 0xFF] ^ (c >>> 8);
    return (c ^ 0xFFFFFFFF) >>> 0;
}

// --- Byte/Bit Conversion ---
function bytesToBits(data) {
    const bits = [];
    for (const b of data) {
        for (let i = 7; i >= 0; i--) bits.push((b >> i) & 1);
    }
    return bits;
}

function bitsToBytes(bits) {
    const bytes = [];
    for (let i = 0; i + 7 < bits.length; i += 8) {
        let b = 0;
        for (let j = 0; j < 8; j++) b = (b << 1) | (bits[i + j] & 1);
        bytes.push(b);
    }
    return new Uint8Array(bytes);
}

// --- Repetition Coding ---
function repeatBits(bits, n) {
    const out = [];
    for (const b of bits) {
        for (let i = 0; i < n; i++) out.push(b);
    }
    return out;
}

function majorityVote(bits, n) {
    const out = [];
    for (let i = 0; i + n - 1 < bits.length; i += n) {
        let sum = 0;
        for (let j = 0; j < n; j++) sum += bits[i + j];
        out.push(sum >= n / 2 ? 1 : 0);
    }
    return out;
}

// --- Frame Building ---
function buildTransmitSignal(fileData, modName, fileName, repetition) {
    repetition = repetition || 1;
    // Encode filename
    const nameBytes = new TextEncoder().encode(fileName || 'file');
    const nameLen = Math.min(nameBytes.length, 255);

    // Packet: [nameLen:1][name:N][dataLen:4][data][CRC-32:4]
    const len = fileData.length;
    const packetSize = 1 + nameLen + 4 + len + 4;
    const payload = new Uint8Array(packetSize);
    let pOff = 0;

    payload[pOff++] = nameLen;
    for (let i = 0; i < nameLen; i++) payload[pOff++] = nameBytes[i];
    payload[pOff++] = (len >> 24) & 0xFF;
    payload[pOff++] = (len >> 16) & 0xFF;
    payload[pOff++] = (len >> 8) & 0xFF;
    payload[pOff++] = len & 0xFF;
    payload.set(fileData, pOff); pOff += len;

    const checksum = crc32(payload.subarray(0, pOff));
    payload[pOff++] = (checksum >> 24) & 0xFF;
    payload[pOff++] = (checksum >> 16) & 0xFF;
    payload[pOff++] = (checksum >> 8) & 0xFF;
    payload[pOff++] = checksum & 0xFF;

    let bits = bytesToBits(payload);
    if (repetition > 1) bits = repeatBits(bits, repetition);
    const { samples, numSymbols, bitsPerSymbol } = modulateOFDM(bits, modName);

    // Build full signal: silence + preamble + CE + data + silence
    const pre1 = generatePreambleSymbol1();
    const pre2 = generatePreambleSymbol2();
    const ce = generateChannelEstSymbol();

    const isAcoustic = OFDM.CP_LEN >= 128;
    const silencePre = new Float32Array(OFDM.SAMPLE_RATE * (isAcoustic ? 0.5 : 0.3));
    const silencePost = new Float32Array(OFDM.SAMPLE_RATE * (isAcoustic ? 0.5 : 0.2));

    let totalLen = silencePre.length + pre1.length + pre2.length + ce.samples.length + silencePost.length;
    for (const s of samples) totalLen += s.length;

    const signal = new Float32Array(totalLen);
    let off = 0;
    signal.set(silencePre, off); off += silencePre.length;
    signal.set(pre1, off); off += pre1.length;
    signal.set(pre2, off); off += pre2.length;
    signal.set(ce.samples, off); off += ce.samples.length;
    for (const s of samples) { signal.set(s, off); off += s.length; }
    signal.set(silencePost, off);

    // Normalize entire signal uniformly (critical for channel estimation)
    let mx = 0;
    for (let i = 0; i < signal.length; i++) mx = Math.max(mx, Math.abs(signal[i]));
    if (mx > 0) { const s = 0.8 / mx; for (let i = 0; i < signal.length; i++) signal[i] *= s; }

    return { signal, numSymbols, bitsPerSymbol, totalBits: bits.length, dataLen: len };
}

function decodeReceivedSignal(signal, modName, repetition) {
    repetition = repetition || 1;
    // Preprocess: DC removal + normalize
    signal = preprocessSignal(signal);

    // Step 1: Coarse preamble detection (Schmidl-Cox auto-correlation, O(n))
    let coarseIdx = detectPreamble(signal);
    if (coarseIdx < 0) return { error: 'Preamble not detected' };

    // Step 2: Fine-tune with cross-correlation around coarse estimate
    const pre1 = generatePreambleSymbol1();
    let tEnergy = 0;
    for (let i = 0; i < pre1.length; i++) tEnergy += pre1[i] * pre1[i];

    const searchRadius = OFDM.CP_LEN * 3;
    const fineStart = Math.max(0, coarseIdx - searchRadius);
    const fineEnd = Math.min(signal.length - pre1.length, coarseIdx + searchRadius);

    let bestMetric = -Infinity, startIdx = coarseIdx;
    for (let d = fineStart; d <= fineEnd; d++) {
        let corr = 0, sEnergy = 0;
        for (let i = 0; i < pre1.length; i++) {
            corr += signal[d + i] * pre1[i];
            sEnergy += signal[d + i] * signal[d + i];
        }
        const denom = Math.sqrt(sEnergy * tEnergy);
        if (denom > 0.001) {
            const metric = corr / denom;
            if (metric > bestMetric) { bestMetric = metric; startIdx = d; }
        }
    }
    if (bestMetric < 0.1) return { error: 'Preamble not detected (low correlation)' };

    // Channel estimation
    const ceStart = startIdx + 2 * OFDM.SYMBOL_LEN;
    if (ceStart + OFDM.SYMBOL_LEN > signal.length) return { error: 'Signal too short for CE' };

    const ceSamples = signal.slice(ceStart, ceStart + OFDM.SYMBOL_LEN);
    const ce = generateChannelEstSymbol();
    const [chRe, chIm] = estimateChannel(ceSamples, ce.knownRe, ce.knownIm);

    // Demodulate data
    const dataStart = ceStart + OFDM.SYMBOL_LEN;
    if (dataStart >= signal.length) return { error: 'No data after CE' };

    const dataSamples = signal.slice(dataStart);
    let bits = demodulateOFDM(dataSamples, modName, chRe, chIm);
    if (repetition > 1) bits = majorityVote(bits, repetition);
    const bytes = bitsToBytes(bits);

    if (bytes.length < 10) return { error: 'Decoded data too short' };

    // Check for chunk frame types (0xFE = metadata, 0xFF = data chunk)
    const firstByte = bytes[0];
    if (firstByte === 0xFE) {
        const result = parseMetadataResult(bytes);
        result.preambleIdx = startIdx;
        return result;
    }
    if (firstByte === 0xFF) {
        const result = parseDataChunkResult(bytes);
        result.preambleIdx = startIdx;
        return result;
    }

    // Legacy packet: [nameLen:1][name:N][dataLen:4][data][CRC-32:4]
    let off = 0;
    const nameLen = bytes[off++];
    if (off + nameLen + 4 + 4 > bytes.length) return { error: 'Decoded data too short for header' };

    let fileName = '';
    try { fileName = new TextDecoder().decode(bytes.slice(off, off + nameLen)); } catch(e) {}
    off += nameLen;

    const dataLen = (bytes[off] << 24) | (bytes[off+1] << 16) | (bytes[off+2] << 8) | bytes[off+3];
    off += 4;

    if (dataLen <= 0 || off + dataLen + 4 > bytes.length) return { error: `Invalid data length: ${dataLen}` };

    const fileData = bytes.slice(off, off + dataLen);
    off += dataLen;

    // Verify CRC
    const expectedCRC = ((bytes[off] << 24) | (bytes[off+1] << 16) |
                          (bytes[off+2] << 8) | bytes[off+3]) >>> 0;
    const actualCRC = crc32(bytes.subarray(0, off));

    return {
        data: fileData,
        dataLen,
        fileName,
        crcValid: expectedCRC === actualCRC,
        expectedCRC,
        actualCRC,
        preambleIdx: startIdx,
        frameType: 'legacy',
    };
}

// ============================================================
// Chunked Transfer Protocol — Large File Support
// ============================================================

// Frame type magic bytes
const FRAME_META = 0xFE;
const FRAME_DATA = 0xFF;

// --- Chunk Frame Payload Builders ---

function buildMetadataPayload(totalChunks, totalFileSize, chunkSize, fileName) {
    const nameBytes = new TextEncoder().encode(fileName || 'file');
    const nameLen = Math.min(nameBytes.length, 255);
    // [0xFE:1][totalChunks:4][totalFileSize:4][chunkSize:2][fileNameLen:1][fileName:N][CRC-32:4]
    const size = 1 + 4 + 4 + 2 + 1 + nameLen + 4;
    const buf = new Uint8Array(size);
    let off = 0;
    buf[off++] = FRAME_META;
    buf[off++] = (totalChunks >> 24) & 0xFF;
    buf[off++] = (totalChunks >> 16) & 0xFF;
    buf[off++] = (totalChunks >> 8) & 0xFF;
    buf[off++] = totalChunks & 0xFF;
    buf[off++] = (totalFileSize >> 24) & 0xFF;
    buf[off++] = (totalFileSize >> 16) & 0xFF;
    buf[off++] = (totalFileSize >> 8) & 0xFF;
    buf[off++] = totalFileSize & 0xFF;
    buf[off++] = (chunkSize >> 8) & 0xFF;
    buf[off++] = chunkSize & 0xFF;
    buf[off++] = nameLen;
    for (let i = 0; i < nameLen; i++) buf[off++] = nameBytes[i];
    const checksum = crc32(buf.subarray(0, off));
    buf[off++] = (checksum >> 24) & 0xFF;
    buf[off++] = (checksum >> 16) & 0xFF;
    buf[off++] = (checksum >> 8) & 0xFF;
    buf[off++] = checksum & 0xFF;
    return buf;
}

function buildDataChunkPayload(chunkData, seqNum) {
    const dataLen = chunkData.length;
    // [0xFF:1][seqNum:4][chunkDataLen:2][data:N][CRC-32:4]
    const size = 1 + 4 + 2 + dataLen + 4;
    const buf = new Uint8Array(size);
    let off = 0;
    buf[off++] = FRAME_DATA;
    buf[off++] = (seqNum >> 24) & 0xFF;
    buf[off++] = (seqNum >> 16) & 0xFF;
    buf[off++] = (seqNum >> 8) & 0xFF;
    buf[off++] = seqNum & 0xFF;
    buf[off++] = (dataLen >> 8) & 0xFF;
    buf[off++] = dataLen & 0xFF;
    buf.set(chunkData, off); off += dataLen;
    const checksum = crc32(buf.subarray(0, off));
    buf[off++] = (checksum >> 24) & 0xFF;
    buf[off++] = (checksum >> 16) & 0xFF;
    buf[off++] = (checksum >> 8) & 0xFF;
    buf[off++] = checksum & 0xFF;
    return buf;
}

// --- Build complete OFDM frames for chunk payloads ---

function buildChunkOFDMFrame(payload, modName, repetition, isFirstFrame) {
    repetition = repetition || 1;
    let bits = bytesToBits(payload);
    if (repetition > 1) bits = repeatBits(bits, repetition);
    const { samples } = modulateOFDM(bits, modName);

    const pre1 = generatePreambleSymbol1();
    const pre2 = generatePreambleSymbol2();
    const ce = generateChannelEstSymbol();

    const isAcoustic = OFDM.CP_LEN >= 128;
    // First frame (metadata) uses longer silence for initial sync
    const silencePreLen = isFirstFrame
        ? Math.round(OFDM.SAMPLE_RATE * (isAcoustic ? 0.5 : 0.3))
        : Math.round(OFDM.SAMPLE_RATE * 0.05);
    const silencePostLen = Math.round(OFDM.SAMPLE_RATE * 0.02);

    const silencePre = new Float32Array(silencePreLen);
    const silencePost = new Float32Array(silencePostLen);

    let totalLen = silencePre.length + pre1.length + pre2.length + ce.samples.length + silencePost.length;
    for (const s of samples) totalLen += s.length;

    const signal = new Float32Array(totalLen);
    let off = 0;
    signal.set(silencePre, off); off += silencePre.length;
    signal.set(pre1, off); off += pre1.length;
    signal.set(pre2, off); off += pre2.length;
    signal.set(ce.samples, off); off += ce.samples.length;
    for (const s of samples) { signal.set(s, off); off += s.length; }
    signal.set(silencePost, off);

    // Normalize
    let mx = 0;
    for (let i = 0; i < signal.length; i++) mx = Math.max(mx, Math.abs(signal[i]));
    if (mx > 0) { const s = 0.8 / mx; for (let i = 0; i < signal.length; i++) signal[i] *= s; }

    return signal;
}

function buildMetadataFrame(totalChunks, totalFileSize, chunkSize, fileName, modName, rep) {
    const payload = buildMetadataPayload(totalChunks, totalFileSize, chunkSize, fileName);
    return buildChunkOFDMFrame(payload, modName, rep, true);
}

function buildDataChunkFrame(chunkData, seqNum, modName, rep) {
    const payload = buildDataChunkPayload(chunkData, seqNum);
    return buildChunkOFDMFrame(payload, modName, rep, false);
}

// --- Decode chunk frame (after preamble detection + CE) ---

function decodeChunkFrame(frameSamples, modName, repetition) {
    repetition = repetition || 1;
    // frameSamples should start from preamble1
    // Structure: [preamble1][preamble2][CE][data symbols...]
    const ceStart = 2 * OFDM.SYMBOL_LEN;
    if (ceStart + OFDM.SYMBOL_LEN > frameSamples.length) {
        return { error: 'Frame too short for CE' };
    }

    const ceSamples = frameSamples.slice(ceStart, ceStart + OFDM.SYMBOL_LEN);
    const ce = generateChannelEstSymbol();
    const [chRe, chIm] = estimateChannel(ceSamples, ce.knownRe, ce.knownIm);

    const dataStart = ceStart + OFDM.SYMBOL_LEN;
    if (dataStart >= frameSamples.length) {
        return { error: 'No data after CE' };
    }

    const dataSamples = frameSamples.slice(dataStart);
    let bits = demodulateOFDM(dataSamples, modName, chRe, chIm);
    if (repetition > 1) bits = majorityVote(bits, repetition);
    const bytes = bitsToBytes(bits);

    if (bytes.length < 6) return { error: 'Decoded data too short' };

    const frameType = bytes[0];
    if (frameType === FRAME_META) {
        return parseMetadataResult(bytes);
    } else if (frameType === FRAME_DATA) {
        return parseDataChunkResult(bytes);
    } else {
        return { error: `Unknown frame type: 0x${frameType.toString(16)}`, frameType };
    }
}

function parseMetadataResult(bytes) {
    // [0xFE:1][totalChunks:4][totalFileSize:4][chunkSize:2][fileNameLen:1][fileName:N][CRC-32:4]
    if (bytes.length < 16) return { error: 'Metadata frame too short' };
    let off = 1;
    const totalChunks = (bytes[off] << 24) | (bytes[off+1] << 16) | (bytes[off+2] << 8) | bytes[off+3]; off += 4;
    const totalFileSize = (bytes[off] << 24) | (bytes[off+1] << 16) | (bytes[off+2] << 8) | bytes[off+3]; off += 4;
    const chunkSize = (bytes[off] << 8) | bytes[off+1]; off += 2;
    const nameLen = bytes[off++];
    if (off + nameLen + 4 > bytes.length) return { error: 'Metadata frame truncated' };
    let fileName = '';
    try { fileName = new TextDecoder().decode(bytes.slice(off, off + nameLen)); } catch(e) {}
    off += nameLen;

    // Verify CRC
    const expectedCRC = ((bytes[off] << 24) | (bytes[off+1] << 16) | (bytes[off+2] << 8) | bytes[off+3]) >>> 0;
    const actualCRC = crc32(bytes.subarray(0, off));

    return {
        frameType: FRAME_META,
        totalChunks, totalFileSize, chunkSize, fileName,
        crcValid: expectedCRC === actualCRC,
        expectedCRC, actualCRC,
    };
}

function parseDataChunkResult(bytes) {
    // [0xFF:1][seqNum:4][chunkDataLen:2][data:N][CRC-32:4]
    if (bytes.length < 11) return { error: 'Data chunk frame too short' };
    let off = 1;
    const seqNum = (bytes[off] << 24) | (bytes[off+1] << 16) | (bytes[off+2] << 8) | bytes[off+3]; off += 4;
    const dataLen = (bytes[off] << 8) | bytes[off+1]; off += 2;
    if (off + dataLen + 4 > bytes.length) return { error: 'Data chunk truncated' };
    const data = bytes.slice(off, off + dataLen);
    off += dataLen;

    const expectedCRC = ((bytes[off] << 24) | (bytes[off+1] << 16) | (bytes[off+2] << 8) | bytes[off+3]) >>> 0;
    const actualCRC = crc32(bytes.subarray(0, off));

    return {
        frameType: FRAME_DATA,
        seqNum, data, dataLen,
        crcValid: expectedCRC === actualCRC,
        expectedCRC, actualCRC,
    };
}

// --- Parse helpers (for external use after raw byte extraction) ---

function parseMetadataPayload(bytes) {
    return parseMetadataResult(bytes);
}

function parseDataChunkPayload(bytes) {
    return parseDataChunkResult(bytes);
}

// --- Estimate frame sample count ---

function estimateFrameSamples(payloadBytes, modName, repetition) {
    repetition = repetition || 1;
    const c = initConstellation(modName);
    const numDataSubs = OFDM.numDataSubs();
    const bitsPerSymbol = numDataSubs * c.bps;
    const totalBits = payloadBytes * 8 * repetition;
    const numSymbols = Math.ceil(totalBits / bitsPerSymbol);

    // preamble1 + preamble2 + CE + data symbols
    const headerSymbols = 3; // pre1 + pre2 + CE
    return (headerSymbols + numSymbols) * OFDM.SYMBOL_LEN;
}

function estimateFrameSamplesWithSilence(payloadBytes, modName, repetition, isFirstFrame) {
    const coreSamples = estimateFrameSamples(payloadBytes, modName, repetition);
    const isAcoustic = OFDM.CP_LEN >= 128;
    const silencePre = isFirstFrame
        ? Math.round(OFDM.SAMPLE_RATE * (isAcoustic ? 0.5 : 0.3))
        : Math.round(OFDM.SAMPLE_RATE * 0.05);
    const silencePost = Math.round(OFDM.SAMPLE_RATE * 0.02);
    return silencePre + coreSamples + silencePost;
}
