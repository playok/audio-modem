package com.example.audiomodem.modem

import kotlin.math.*

/**
 * FFT/IFFT implementation using Cooley-Tukey radix-2 algorithm.
 * Complex numbers are represented as pairs of DoubleArrays (real, imag).
 */
object FFT {

    /**
     * Compute FFT. Input length must be a power of 2.
     */
    fun fft(real: DoubleArray, imag: DoubleArray): Pair<DoubleArray, DoubleArray> {
        val n = real.size
        require(n > 0 && n and (n - 1) == 0) { "Length must be a power of 2" }

        val re = real.copyOf()
        val im = imag.copyOf()

        bitReverse(re, im)
        fftIterative(re, im, false)

        return Pair(re, im)
    }

    /**
     * Compute IFFT. Input length must be a power of 2.
     */
    fun ifft(real: DoubleArray, imag: DoubleArray): Pair<DoubleArray, DoubleArray> {
        val n = real.size
        require(n > 0 && n and (n - 1) == 0) { "Length must be a power of 2" }

        val re = real.copyOf()
        val im = imag.copyOf()

        bitReverse(re, im)
        fftIterative(re, im, true)

        // Scale by 1/N
        val scale = 1.0 / n
        for (i in 0 until n) {
            re[i] *= scale
            im[i] *= scale
        }

        return Pair(re, im)
    }

    /**
     * FFT of real-valued input.
     */
    fun realFft(x: DoubleArray): Pair<DoubleArray, DoubleArray> {
        return fft(x, DoubleArray(x.size))
    }

    /**
     * IFFT returning only the real part.
     */
    fun realIfft(real: DoubleArray, imag: DoubleArray): DoubleArray {
        val (re, _) = ifft(real, imag)
        return re
    }

    private fun fftIterative(re: DoubleArray, im: DoubleArray, inverse: Boolean) {
        val n = re.size
        var size = 2
        while (size <= n) {
            val halfSize = size / 2
            val sign = if (inverse) 1.0 else -1.0
            val angle = sign * 2.0 * PI / size

            var start = 0
            while (start < n) {
                var wRe = 1.0
                var wIm = 0.0
                val wnRe = cos(angle)
                val wnIm = sin(angle)

                for (j in 0 until halfSize) {
                    val idx1 = start + j
                    val idx2 = start + j + halfSize

                    val tRe = wRe * re[idx2] - wIm * im[idx2]
                    val tIm = wRe * im[idx2] + wIm * re[idx2]

                    re[idx2] = re[idx1] - tRe
                    im[idx2] = im[idx1] - tIm
                    re[idx1] += tRe
                    im[idx1] += tIm

                    val newWRe = wRe * wnRe - wIm * wnIm
                    val newWIm = wRe * wnIm + wIm * wnRe
                    wRe = newWRe
                    wIm = newWIm
                }

                start += size
            }
            size *= 2
        }
    }

    private fun bitReverse(re: DoubleArray, im: DoubleArray) {
        val n = re.size
        var bits = 0
        var tmp = n
        while (tmp > 1) {
            bits++
            tmp = tmp shr 1
        }

        for (i in 0 until n) {
            val j = reverseBits(i, bits)
            if (i < j) {
                var temp = re[i]; re[i] = re[j]; re[j] = temp
                temp = im[i]; im[i] = im[j]; im[j] = temp
            }
        }
    }

    private fun reverseBits(x: Int, bits: Int): Int {
        var result = 0
        var v = x
        for (i in 0 until bits) {
            result = (result shl 1) or (v and 1)
            v = v shr 1
        }
        return result
    }
}
