package com.example.audiomodem.audio

import android.media.AudioFormat
import android.media.AudioRecord
import android.media.AudioTrack
import android.media.MediaRecorder

/**
 * AudioIO wraps Android's AudioRecord and AudioTrack for audio I/O.
 * Uses VOICE_RECOGNITION source to bypass AGC and noise suppression.
 */
class AudioIO {
    companion object {
        const val SAMPLE_RATE = 44100
        const val CHANNEL_IN = AudioFormat.CHANNEL_IN_MONO
        const val CHANNEL_OUT = AudioFormat.CHANNEL_OUT_MONO
        const val ENCODING = AudioFormat.ENCODING_PCM_FLOAT
        const val FRAMES_PER_BUF = 576 // OFDM symbol length
    }

    private var audioRecord: AudioRecord? = null
    private var audioTrack: AudioTrack? = null

    /**
     * Initialize the audio input (microphone).
     */
    fun openInput(): Boolean {
        val minBufSize = AudioRecord.getMinBufferSize(SAMPLE_RATE, CHANNEL_IN, ENCODING)
        val bufSize = maxOf(minBufSize, FRAMES_PER_BUF * 4)

        audioRecord = AudioRecord(
            MediaRecorder.AudioSource.VOICE_RECOGNITION,
            SAMPLE_RATE,
            CHANNEL_IN,
            ENCODING,
            bufSize
        )

        return audioRecord?.state == AudioRecord.STATE_INITIALIZED
    }

    /**
     * Initialize the audio output (speaker/headphone).
     */
    fun openOutput(): Boolean {
        val minBufSize = AudioTrack.getMinBufferSize(SAMPLE_RATE, CHANNEL_OUT, ENCODING)
        val bufSize = maxOf(minBufSize, FRAMES_PER_BUF * 4)

        audioTrack = AudioTrack.Builder()
            .setAudioFormat(
                AudioFormat.Builder()
                    .setSampleRate(SAMPLE_RATE)
                    .setChannelMask(CHANNEL_OUT)
                    .setEncoding(ENCODING)
                    .build()
            )
            .setBufferSizeInBytes(bufSize)
            .setTransferMode(AudioTrack.MODE_STREAM)
            .build()

        return audioTrack?.state == AudioTrack.STATE_INITIALIZED
    }

    fun startInput() {
        audioRecord?.startRecording()
    }

    fun stopInput() {
        audioRecord?.stop()
    }

    fun startOutput() {
        audioTrack?.play()
    }

    fun stopOutput() {
        audioTrack?.stop()
    }

    /**
     * Read audio samples from the microphone.
     */
    fun read(numSamples: Int): FloatArray {
        val buffer = FloatArray(numSamples)
        audioRecord?.read(buffer, 0, numSamples, AudioRecord.READ_BLOCKING)
        return buffer
    }

    /**
     * Write audio samples to the speaker.
     */
    fun write(samples: FloatArray): Int {
        return audioTrack?.write(samples, 0, samples.size, AudioTrack.WRITE_BLOCKING) ?: 0
    }

    /**
     * Write samples in chunks matching the OFDM symbol size.
     */
    fun writeSamples(samples: FloatArray) {
        var offset = 0
        while (offset < samples.size) {
            val remaining = samples.size - offset
            val chunkSize = minOf(remaining, FRAMES_PER_BUF)
            val chunk = samples.copyOfRange(offset, offset + chunkSize)

            if (chunk.size < FRAMES_PER_BUF) {
                val padded = FloatArray(FRAMES_PER_BUF)
                chunk.copyInto(padded)
                write(padded)
            } else {
                write(chunk)
            }
            offset += chunkSize
        }
    }

    /**
     * Read a specified number of samples from input.
     */
    fun readSamples(numSamples: Int): FloatArray {
        val result = FloatArray(numSamples)
        var offset = 0
        while (offset < numSamples) {
            val remaining = numSamples - offset
            val chunkSize = minOf(remaining, FRAMES_PER_BUF)
            val chunk = read(chunkSize)
            chunk.copyInto(result, offset)
            offset += chunkSize
        }
        return result
    }

    fun close() {
        audioRecord?.release()
        audioRecord = null
        audioTrack?.release()
        audioTrack = null
    }
}
