// Writes public/beep.wav: 880 Hz sine, 120 ms, 16-bit mono PCM.
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const SAMPLE_RATE = 44100;
const FREQUENCY_HZ = 880;
const DURATION_MS = 120;
const AMPLITUDE = 0.5;
const FADE_SAMPLES = Math.round(SAMPLE_RATE * 0.005);

const sampleCount = Math.round((SAMPLE_RATE * DURATION_MS) / 1000);
const dataSize = sampleCount * 2;
const buffer = Buffer.alloc(44 + dataSize);

buffer.write('RIFF', 0, 'ascii');
buffer.writeUInt32LE(36 + dataSize, 4);
buffer.write('WAVE', 8, 'ascii');
buffer.write('fmt ', 12, 'ascii');
buffer.writeUInt32LE(16, 16); // fmt chunk size
buffer.writeUInt16LE(1, 20); // PCM
buffer.writeUInt16LE(1, 22); // mono
buffer.writeUInt32LE(SAMPLE_RATE, 24);
buffer.writeUInt32LE(SAMPLE_RATE * 2, 28); // byte rate
buffer.writeUInt16LE(2, 32); // block align
buffer.writeUInt16LE(16, 34); // bits per sample
buffer.write('data', 36, 'ascii');
buffer.writeUInt32LE(dataSize, 40);

for (let i = 0; i < sampleCount; i++) {
	// Short fade in and out to avoid clicks.
	const fade = Math.min(1, i / FADE_SAMPLES, (sampleCount - 1 - i) / FADE_SAMPLES);
	const value = Math.sin((2 * Math.PI * FREQUENCY_HZ * i) / SAMPLE_RATE) * AMPLITUDE * fade;
	buffer.writeInt16LE(Math.round(value * 32767), 44 + i * 2);
}

const publicDir = join(dirname(fileURLToPath(import.meta.url)), '..', 'public');
mkdirSync(publicDir, { recursive: true });
writeFileSync(join(publicDir, 'beep.wav'), buffer);
console.log(`gen:beep: public/beep.wav (${buffer.length} bytes)`);
