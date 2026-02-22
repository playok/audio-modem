package com.example.audiomodem.modem

import kotlin.math.*
import kotlin.random.Random

/**
 * OFDM modem implementing modulation and demodulation.
 * Protocol-compatible with the Go PC implementation.
 */
object OFDMParams {
    const val FFT_SIZE = 512
    const val CP_LEN = 64
    const val SYMBOL_LEN = FFT_SIZE + CP_LEN // 576
    const val SAMPLE_RATE = 44100
    const val SUBCARRIER_START = 12  // ~1032 Hz
    const val SUBCARRIER_END = 232   // ~19992 Hz
    const val NUM_PILOTS = 16

    val PILOT_PATTERN = intArrayOf(15, 29, 43, 57, 71, 85, 99, 113, 127, 141, 155, 169, 183, 197, 211, 225)

    fun isPilot(idx: Int): Boolean = idx in PILOT_PATTERN
    fun numDataSubcarriers(): Int = (SUBCARRIER_START..SUBCARRIER_END).count { !isPilot(it) }
    fun bitsPerOFDMSymbol(mod: Modulation): Int = numDataSubcarriers() * mod.bitsPerSymbol
}

class OFDMModulator(private val mod: Modulation) {
    private val constellation = Constellation(mod)

    fun modulateSingle(bits: ByteArray): FloatArray {
        val (dataRe, dataIm) = constellation.mapBits(bits)

        // Build spectrum with pilots
        val specRe = DoubleArray(OFDMParams.FFT_SIZE)
        val specIm = DoubleArray(OFDMParams.FFT_SIZE)

        var dataIdx = 0
        for (k in OFDMParams.SUBCARRIER_START..OFDMParams.SUBCARRIER_END) {
            if (OFDMParams.isPilot(k)) {
                specRe[k] = 1.0 // Pilot = +1
                specIm[k] = 0.0
            } else if (dataIdx < dataRe.size) {
                specRe[k] = dataRe[dataIdx]
                specIm[k] = dataIm[dataIdx]
                dataIdx++
            }
        }

        // Hermitian symmetry
        val n = OFDMParams.FFT_SIZE
        for (k in 1 until n / 2) {
            specRe[n - k] = specRe[k]
            specIm[n - k] = -specIm[k]
        }
        specRe[0] = 0.0; specIm[0] = 0.0
        specIm[n / 2] = 0.0

        // IFFT
        val timeDomain = FFT.realIfft(specRe, specIm)

        // Add cyclic prefix
        val withCP = DoubleArray(OFDMParams.SYMBOL_LEN)
        System.arraycopy(timeDomain, n - OFDMParams.CP_LEN, withCP, 0, OFDMParams.CP_LEN)
        System.arraycopy(timeDomain, 0, withCP, OFDMParams.CP_LEN, n)

        // Normalize
        val maxAbs = withCP.maxOf { abs(it) }.coerceAtLeast(1e-10)
        val scale = 0.8 / maxAbs
        return FloatArray(withCP.size) { (withCP[it] * scale).toFloat() }
    }

    fun modulate(bits: ByteArray): FloatArray {
        val bps = OFDMParams.bitsPerOFDMSymbol(mod)
        val numSymbols = bits.size / bps
        val result = FloatArray(numSymbols * OFDMParams.SYMBOL_LEN)

        for (i in 0 until numSymbols) {
            val symBits = bits.copyOfRange(i * bps, (i + 1) * bps)
            val samples = modulateSingle(symBits)
            samples.copyInto(result, i * OFDMParams.SYMBOL_LEN)
        }
        return result
    }
}

class OFDMDemodulator(private val mod: Modulation) {
    private val constellation = Constellation(mod)
    private var channelRe = DoubleArray(OFDMParams.FFT_SIZE)
    private var channelIm = DoubleArray(OFDMParams.FFT_SIZE)

    fun setChannelEstimate(recRe: DoubleArray, recIm: DoubleArray,
                            knownRe: DoubleArray, knownIm: DoubleArray) {
        channelRe = DoubleArray(OFDMParams.FFT_SIZE)
        channelIm = DoubleArray(OFDMParams.FFT_SIZE)

        for (k in OFDMParams.SUBCARRIER_START..OFDMParams.SUBCARRIER_END) {
            val kr = knownRe[k]; val ki = knownIm[k]
            val denom = kr * kr + ki * ki
            if (denom > 1e-10) {
                // H(k) = Y(k) / X(k) = (yr + j*yi) / (xr + j*xi)
                channelRe[k] = (recRe[k] * kr + recIm[k] * ki) / denom
                channelIm[k] = (recIm[k] * kr - recRe[k] * ki) / denom
            }
        }
    }

    fun demodulateSingle(samples: FloatArray): ByteArray {
        // Remove CP
        val withoutCP = DoubleArray(OFDMParams.FFT_SIZE)
        for (i in 0 until OFDMParams.FFT_SIZE) {
            withoutCP[i] = samples[i + OFDMParams.CP_LEN].toDouble()
        }

        // FFT
        val (specRe, specIm) = FFT.realFft(withoutCP)

        // Equalize
        val eqRe = DoubleArray(OFDMParams.FFT_SIZE)
        val eqIm = DoubleArray(OFDMParams.FFT_SIZE)
        for (k in OFDMParams.SUBCARRIER_START..OFDMParams.SUBCARRIER_END) {
            val hr = channelRe[k]; val hi = channelIm[k]
            val hMag = hr * hr + hi * hi
            if (hMag > 1e-10) {
                // Y/H = Y * conj(H) / |H|^2
                eqRe[k] = (specRe[k] * hr + specIm[k] * hi) / hMag
                eqIm[k] = (specIm[k] * hr - specRe[k] * hi) / hMag
            } else {
                eqRe[k] = specRe[k]
                eqIm[k] = specIm[k]
            }
        }

        // Extract pilots for phase correction
        var phaseSum = 0.0
        var pilotCount = 0
        for (p in OFDMParams.PILOT_PATTERN) {
            if (p in OFDMParams.SUBCARRIER_START..OFDMParams.SUBCARRIER_END) {
                val re = eqRe[p]; val im = eqIm[p]
                if (re != 0.0) {
                    phaseSum += im / re
                    pilotCount++
                }
            }
        }
        val phaseOffset = if (pilotCount > 0) phaseSum / pilotCount else 0.0

        // Extract data and apply phase correction
        val dataRe = mutableListOf<Double>()
        val dataIm = mutableListOf<Double>()
        for (k in OFDMParams.SUBCARRIER_START..OFDMParams.SUBCARRIER_END) {
            if (!OFDMParams.isPilot(k)) {
                // Simple phase correction: rotate by -phaseOffset
                val corrRe = eqRe[k] + eqIm[k] * phaseOffset
                val corrIm = eqIm[k] - eqRe[k] * phaseOffset
                dataRe.add(corrRe)
                dataIm.add(corrIm)
            }
        }

        return constellation.demapSymbols(dataRe.toDoubleArray(), dataIm.toDoubleArray())
    }

    fun demodulate(samples: FloatArray): ByteArray {
        val numSymbols = samples.size / OFDMParams.SYMBOL_LEN
        val allBits = mutableListOf<Byte>()
        for (i in 0 until numSymbols) {
            val sym = samples.copyOfRange(i * OFDMParams.SYMBOL_LEN, (i + 1) * OFDMParams.SYMBOL_LEN)
            allBits.addAll(demodulateSingle(sym).toList())
        }
        return allBits.toByteArray()
    }
}

/**
 * Schmidl-Cox preamble generation and detection.
 */
object Preamble {
    private const val DETECTION_THRESHOLD = 0.7

    fun generateSchmidlCox(): Pair<FloatArray, FloatArray> {
        // Symbol 1: even subcarriers only
        val specRe1 = DoubleArray(OFDMParams.FFT_SIZE)
        val specIm1 = DoubleArray(OFDMParams.FFT_SIZE)
        val rng1 = Random(42)
        for (k in OFDMParams.SUBCARRIER_START..OFDMParams.SUBCARRIER_END step 2) {
            specRe1[k] = if (rng1.nextInt(2) == 0) 1.0 else -1.0
        }
        // Hermitian symmetry
        val n = OFDMParams.FFT_SIZE
        for (k in 1 until n / 2) {
            specRe1[n - k] = specRe1[k]
            specIm1[n - k] = -specIm1[k]
        }
        specRe1[0] = 0.0; specRe1[n / 2] = 0.0

        val td1 = FFT.realIfft(specRe1, specIm1)
        val sym1 = addCPAndNormalize(td1)

        // Symbol 2: all subcarriers
        val specRe2 = DoubleArray(OFDMParams.FFT_SIZE)
        val specIm2 = DoubleArray(OFDMParams.FFT_SIZE)
        val rng2 = Random(43)
        for (k in OFDMParams.SUBCARRIER_START..OFDMParams.SUBCARRIER_END) {
            specRe2[k] = if (rng2.nextInt(2) == 0) 1.0 else -1.0
        }
        for (k in 1 until n / 2) {
            specRe2[n - k] = specRe2[k]
            specIm2[n - k] = -specIm2[k]
        }
        specRe2[0] = 0.0; specRe2[n / 2] = 0.0

        val td2 = FFT.realIfft(specRe2, specIm2)
        val sym2 = addCPAndNormalize(td2)

        return Pair(sym1, sym2)
    }

    fun generateChannelEstimation(): Triple<FloatArray, DoubleArray, DoubleArray> {
        val specRe = DoubleArray(OFDMParams.FFT_SIZE)
        val specIm = DoubleArray(OFDMParams.FFT_SIZE)
        val knownRe = DoubleArray(OFDMParams.FFT_SIZE)
        val knownIm = DoubleArray(OFDMParams.FFT_SIZE)

        val rng = Random(44)
        val n = OFDMParams.FFT_SIZE
        for (k in OFDMParams.SUBCARRIER_START..OFDMParams.SUBCARRIER_END) {
            val v = if (rng.nextInt(2) == 0) 1.0 else -1.0
            specRe[k] = v
            knownRe[k] = v
        }
        for (k in 1 until n / 2) {
            specRe[n - k] = specRe[k]
            specIm[n - k] = -specIm[k]
        }
        specRe[0] = 0.0; specRe[n / 2] = 0.0

        val td = FFT.realIfft(specRe, specIm)
        val samples = addCPAndNormalize(td)

        return Triple(samples, knownRe, knownIm)
    }

    fun detect(signal: FloatArray): Int {
        val halfLen = OFDMParams.FFT_SIZE / 2
        val symbolLen = OFDMParams.SYMBOL_LEN

        if (signal.size < symbolLen + halfLen) return -1

        var bestMetric = 0.0
        var bestIdx = -1

        for (d in 0 until signal.size - symbolLen) {
            var pReal = 0.0
            var rr = 0.0

            for (m in 0 until halfLen) {
                if (d + m + halfLen >= signal.size) break
                val a = signal[d + m].toDouble()
                val b = signal[d + m + halfLen].toDouble()
                pReal += a * b
                rr += b * b
            }

            if (rr > 0) {
                val metric = (pReal * pReal) / (rr * rr)
                if (metric > bestMetric) {
                    bestMetric = metric
                    bestIdx = d
                }
            }
        }

        return if (bestMetric > DETECTION_THRESHOLD) bestIdx else -1
    }

    private fun addCPAndNormalize(timeDomain: DoubleArray): FloatArray {
        val n = timeDomain.size
        val cp = OFDMParams.CP_LEN
        val result = DoubleArray(cp + n)
        System.arraycopy(timeDomain, n - cp, result, 0, cp)
        System.arraycopy(timeDomain, 0, result, cp, n)

        val maxAbs = result.maxOf { abs(it) }.coerceAtLeast(1e-10)
        val scale = 0.8 / maxAbs
        return FloatArray(result.size) { (result[it] * scale).toFloat() }
    }
}
