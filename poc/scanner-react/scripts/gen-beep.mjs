// Writes public/beep.wav: 880 Hz sine, 120 ms, 16-bit PCM mono.
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

const sampleRate = 44100;
const frequency = 880;
const durationMs = 120;
const fadeMs = 5;
const amplitude = 0.5;

const samples = Math.round((sampleRate * durationMs) / 1000);
const fadeSamples = Math.round((sampleRate * fadeMs) / 1000);
const dataSize = samples * 2;
const buffer = Buffer.alloc(44 + dataSize);

buffer.write("RIFF", 0, "ascii");
buffer.writeUInt32LE(36 + dataSize, 4);
buffer.write("WAVE", 8, "ascii");
buffer.write("fmt ", 12, "ascii");
buffer.writeUInt32LE(16, 16); // fmt chunk size
buffer.writeUInt16LE(1, 20); // PCM
buffer.writeUInt16LE(1, 22); // mono
buffer.writeUInt32LE(sampleRate, 24);
buffer.writeUInt32LE(sampleRate * 2, 28); // byte rate
buffer.writeUInt16LE(2, 32); // block align
buffer.writeUInt16LE(16, 34); // bits per sample
buffer.write("data", 36, "ascii");
buffer.writeUInt32LE(dataSize, 40);

for (let i = 0; i < samples; i++) {
  const fade = Math.min(1, i / fadeSamples, (samples - 1 - i) / fadeSamples);
  const value = Math.sin((2 * Math.PI * frequency * i) / sampleRate) * amplitude * fade;
  buffer.writeInt16LE(Math.round(value * 32767), 44 + i * 2);
}

const target = fileURLToPath(new URL("../public/beep.wav", import.meta.url));
mkdirSync(dirname(target), { recursive: true });
writeFileSync(target, buffer);
console.log(`gen:beep: ${target} (${buffer.length} bytes)`);
