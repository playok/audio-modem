package com.example.audiomodem.modem

import kotlin.math.sqrt

/**
 * QAM constellation mapping and demapping.
 */
enum class Modulation(val bitsPerSymbol: Int) {
    QPSK(2),
    QAM16(4),
    QAM64(6);

    override fun toString(): String = when (this) {
        QPSK -> "QPSK"
        QAM16 -> "16-QAM"
        QAM64 -> "64-QAM"
    }
}

class Constellation(val mod: Modulation) {
    // Points stored as interleaved [re0, im0, re1, im1, ...]
    private val pointsRe: DoubleArray
    private val pointsIm: DoubleArray

    init {
        val (re, im) = when (mod) {
            Modulation.QPSK -> generateQPSK()
            Modulation.QAM16 -> generateQAM(4)
            Modulation.QAM64 -> generateQAM(8)
        }
        // Normalize to unit average power
        var avgPower = 0.0
        for (i in re.indices) {
            avgPower += re[i] * re[i] + im[i] * im[i]
        }
        avgPower /= re.size
        val scale = 1.0 / sqrt(avgPower)
        for (i in re.indices) {
            re[i] *= scale
            im[i] *= scale
        }
        pointsRe = re
        pointsIm = im
    }

    private fun generateQPSK(): Pair<DoubleArray, DoubleArray> {
        val re = doubleArrayOf(1.0, -1.0, -1.0, 1.0)
        val im = doubleArrayOf(1.0, 1.0, -1.0, -1.0)
        return Pair(re, im)
    }

    private fun generateQAM(order: Int): Pair<DoubleArray, DoubleArray> {
        val size = order * order
        val re = DoubleArray(size)
        val im = DoubleArray(size)

        for (i in 0 until size) {
            val row = i / order
            val col = i % order
            val grayRow = row xor (row shr 1)
            val grayCol = col xor (col shr 1)
            re[i] = (2.0 * grayCol - order + 1)
            im[i] = (2.0 * grayRow - order + 1)
        }
        return Pair(re, im)
    }

    fun map(bits: ByteArray): Pair<Double, Double> {
        val idx = bitsToIndex(bits)
        return Pair(pointsRe[idx.coerceIn(0, pointsRe.size - 1)],
                    pointsIm[idx.coerceIn(0, pointsIm.size - 1)])
    }

    fun demap(re: Double, im: Double): ByteArray {
        var minDist = Double.MAX_VALUE
        var minIdx = 0
        for (i in pointsRe.indices) {
            val dr = re - pointsRe[i]
            val di = im - pointsIm[i]
            val d = dr * dr + di * di
            if (d < minDist) {
                minDist = d
                minIdx = i
            }
        }
        return indexToBits(minIdx, mod.bitsPerSymbol)
    }

    fun mapBits(bits: ByteArray): Pair<DoubleArray, DoubleArray> {
        val bps = mod.bitsPerSymbol
        val numSymbols = bits.size / bps
        val re = DoubleArray(numSymbols)
        val im = DoubleArray(numSymbols)

        for (i in 0 until numSymbols) {
            val subBits = bits.copyOfRange(i * bps, (i + 1) * bps)
            val (r, ii) = map(subBits)
            re[i] = r
            im[i] = ii
        }
        return Pair(re, im)
    }

    fun demapSymbols(re: DoubleArray, im: DoubleArray): ByteArray {
        val bps = mod.bitsPerSymbol
        val bits = ByteArray(re.size * bps)
        for (i in re.indices) {
            val b = demap(re[i], im[i])
            b.copyInto(bits, i * bps)
        }
        return bits
    }

    companion object {
        fun bitsToIndex(bits: ByteArray): Int {
            var idx = 0
            for (b in bits) {
                idx = (idx shl 1) or (b.toInt() and 1)
            }
            return idx
        }

        fun indexToBits(idx: Int, numBits: Int): ByteArray {
            val bits = ByteArray(numBits)
            var v = idx
            for (i in numBits - 1 downTo 0) {
                bits[i] = (v and 1).toByte()
                v = v shr 1
            }
            return bits
        }
    }
}
