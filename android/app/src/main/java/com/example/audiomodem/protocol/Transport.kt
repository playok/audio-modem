package com.example.audiomodem.protocol

import android.util.Log
import com.example.audiomodem.audio.AudioIO
import com.example.audiomodem.modem.*
import kotlinx.coroutines.*

/**
 * Transport layer implementing Stop-and-Wait ARQ over the audio modem.
 */
class Transport(
    private val audioIO: AudioIO,
    private val mod: Modulation
) {
    companion object {
        private const val TAG = "Transport"
        const val ACK_TIMEOUT_MS = 500L
        const val MAX_RETRIES = 3
        const val TURNAROUND_MS = 50L
    }

    private val modulator = OFDMModulator(mod)
    private val demodulator = OFDMDemodulator(mod)
    private var seqNum: Byte = 0

    var onProgress: ((sent: Long, total: Long, message: String) -> Unit)? = null

    /**
     * Send a frame and wait for ACK.
     */
    suspend fun sendFrame(frame: Frame): Boolean {
        val f = frame.copy(seqNum = seqNum)

        for (retry in 0..MAX_RETRIES) {
            if (retry > 0) Log.d(TAG, "Retry $retry for seq=$seqNum")

            // Modulate and send
            val frameBytes = f.encode()
            val bits = bytesToBits(frameBytes)
            val bps = OFDMParams.bitsPerOFDMSymbol(mod)
            val paddedBits = if (bits.size % bps != 0) {
                bits + ByteArray(bps - bits.size % bps)
            } else bits

            // Generate preamble + channel estimation + data
            val (pre1, pre2) = Preamble.generateSchmidlCox()
            val (ce, _, _) = Preamble.generateChannelEstimation()
            val dataSamples = modulator.modulate(paddedBits)

            val signal = FloatArray(pre1.size + pre2.size + ce.size + dataSamples.size)
            pre1.copyInto(signal, 0)
            pre2.copyInto(signal, pre1.size)
            ce.copyInto(signal, pre1.size + pre2.size)
            dataSamples.copyInto(signal, pre1.size + pre2.size + ce.size)

            audioIO.startOutput()
            audioIO.writeSamples(signal)
            audioIO.stopOutput()

            delay(TURNAROUND_MS)

            // Wait for ACK
            val ackFrame = receiveFrameInternal(ACK_TIMEOUT_MS) ?: continue

            if (ackFrame.type == FrameType.ACK && ackFrame.seqNum == seqNum) {
                seqNum++
                return true
            }
        }

        Log.e(TAG, "Max retries exceeded for seq=$seqNum")
        return false
    }

    /**
     * Wait for and receive a frame, then send ACK.
     */
    suspend fun receiveFrame(timeoutMs: Long): Frame? {
        val frame = receiveFrameInternal(timeoutMs) ?: return null

        delay(TURNAROUND_MS)

        // Send ACK
        val ack = Frame.ackFrame(frame.seqNum)
        sendFrameRaw(ack)

        return frame
    }

    /**
     * Perform PING/PONG handshake (initiator side).
     */
    suspend fun handshake(): Boolean {
        val ping = Frame.pingFrame()
        sendFrameRaw(ping)
        delay(TURNAROUND_MS)

        val pong = receiveFrameInternal(2 * ACK_TIMEOUT_MS) ?: return false
        return pong.type == FrameType.PONG
    }

    /**
     * Wait for handshake (responder side).
     */
    suspend fun waitForHandshake(timeoutMs: Long): Boolean {
        val ping = receiveFrameInternal(timeoutMs) ?: return false
        if (ping.type != FrameType.PING) return false

        delay(TURNAROUND_MS)
        sendFrameRaw(Frame.pongFrame())
        return true
    }

    private suspend fun receiveFrameInternal(timeoutMs: Long): Frame? {
        audioIO.startInput()

        val totalSamples = 14 * OFDMParams.SYMBOL_LEN
        val deadline = System.currentTimeMillis() + timeoutMs
        val allSamples = mutableListOf<Float>()

        while (System.currentTimeMillis() < deadline && allSamples.size < totalSamples) {
            val chunk = audioIO.read(OFDMParams.SYMBOL_LEN)
            allSamples.addAll(chunk.toList())
            yield()
        }

        audioIO.stopInput()

        if (allSamples.size < 4 * OFDMParams.SYMBOL_LEN) return null

        val signal = allSamples.toFloatArray()

        // Detect preamble
        val startIdx = Preamble.detect(signal)
        if (startIdx < 0) return null

        // Skip preamble (2 symbols) → channel estimation
        val ceStart = startIdx + 2 * OFDMParams.SYMBOL_LEN
        if (ceStart + OFDMParams.SYMBOL_LEN > signal.size) return null

        val ceSamples = DoubleArray(OFDMParams.FFT_SIZE) {
            signal[ceStart + OFDMParams.CP_LEN + it].toDouble()
        }
        val (ceRe, ceIm) = FFT.realFft(ceSamples)

        val (_, knownRe, knownIm) = Preamble.generateChannelEstimation()
        demodulator.setChannelEstimate(ceRe, ceIm, knownRe, knownIm)

        // Demodulate data
        val dataStart = ceStart + OFDMParams.SYMBOL_LEN
        if (dataStart >= signal.size) return null

        val dataSamples = signal.copyOfRange(dataStart, signal.size)
        val bits = demodulator.demodulate(dataSamples)
        val bytes = bitsToBytes(bits)

        return try {
            Frame.decode(bytes)
        } catch (e: Exception) {
            Log.e(TAG, "Frame decode error: ${e.message}")
            null
        }
    }

    private fun sendFrameRaw(frame: Frame) {
        val frameBytes = frame.encode()
        val bits = bytesToBits(frameBytes)
        val bps = OFDMParams.bitsPerOFDMSymbol(mod)
        val paddedBits = if (bits.size % bps != 0) {
            bits + ByteArray(bps - bits.size % bps)
        } else bits

        val (pre1, pre2) = Preamble.generateSchmidlCox()
        val (ce, _, _) = Preamble.generateChannelEstimation()
        val dataSamples = modulator.modulate(paddedBits)

        val signal = FloatArray(pre1.size + pre2.size + ce.size + dataSamples.size)
        pre1.copyInto(signal, 0)
        pre2.copyInto(signal, pre1.size)
        ce.copyInto(signal, pre1.size + pre2.size)
        dataSamples.copyInto(signal, pre1.size + pre2.size + ce.size)

        audioIO.startOutput()
        audioIO.writeSamples(signal)
        audioIO.stopOutput()
    }

    fun reset() {
        seqNum = 0
    }
}

fun bytesToBits(data: ByteArray): ByteArray {
    val bits = ByteArray(data.size * 8)
    for (i in data.indices) {
        for (j in 7 downTo 0) {
            bits[i * 8 + (7 - j)] = ((data[i].toInt() shr j) and 1).toByte()
        }
    }
    return bits
}

fun bitsToBytes(bits: ByteArray): ByteArray {
    val numBytes = bits.size / 8
    val data = ByteArray(numBytes)
    for (i in 0 until numBytes) {
        var b = 0
        for (j in 0 until 8) {
            b = (b shl 1) or (bits[i * 8 + j].toInt() and 1)
        }
        data[i] = b.toByte()
    }
    return data
}
