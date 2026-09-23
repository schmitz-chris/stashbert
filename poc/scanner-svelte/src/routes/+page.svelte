<script lang="ts">
	import { asset } from '$app/paths';
	import { MediaQuery } from 'svelte/reactivity';
	import { stripRegion } from '$lib/scanner/decoder';
	import { Scanner } from '$lib/scanner/scanner.svelte';

	const scanner = new Scanner(asset('/zxing_reader.wasm'));
	const standalone = new MediaQuery('display-mode: standalone');
	// The decoded strip in percent of the video, drawn as a frame over it.
	const frame = stripRegion(100, 100);
	const timeFormat = new Intl.DateTimeFormat('de-DE', { timeStyle: 'medium' });
</script>

<svelte:document onvisibilitychange={scanner.handleVisibilityChange} />

<main class="mx-auto flex max-w-xl flex-col gap-4 p-4">
	<h1 class="text-xl font-semibold">Scanner-Test S</h1>

	{#if scanner.phase === 'idle'}
		<button
			class="min-h-20 rounded-xl bg-blue-600 px-4 text-2xl font-semibold text-white"
			onclick={scanner.start}
		>
			Scannen starten
		</button>
	{:else if scanner.phase === 'paused'}
		<button
			class="min-h-20 rounded-xl bg-amber-500 px-4 text-2xl font-semibold text-black"
			onclick={scanner.resume}
		>
			Tippen zum Fortsetzen
		</button>
	{/if}

	<div class="relative mx-auto w-fit bg-black">
		<video
			class="block h-auto max-h-[60vh] w-auto max-w-full"
			autoplay
			muted
			playsinline
			{@attach scanner.attachVideo}
		></video>
		<div
			class="pointer-events-none absolute border-2 border-red-500"
			style:left="{frame.x}%"
			style:top="{frame.y}%"
			style:width="{frame.width}%"
			style:height="{frame.height}%"
		></div>
		{#if scanner.flash}
			<div class="pointer-events-none absolute inset-0 bg-green-500/60"></div>
		{/if}
	</div>

	<section class="flex flex-col gap-3">
		{#if scanner.cameras.length > 0}
			<label class="flex flex-col gap-1">
				<span>Kamera</span>
				<select
					class="min-h-11 rounded border border-gray-400 px-2"
					disabled={scanner.phase === 'starting'}
					bind:value={() => scanner.cameraId, (id) => scanner.selectCamera(id)}
				>
					{#each scanner.cameras as camera, index (camera.deviceId)}
						<option value={camera.deviceId}>{camera.label || `Kamera ${index + 1}`}</option>
					{/each}
				</select>
			</label>
		{/if}

		{#if scanner.zoom}
			<label class="flex flex-col gap-1">
				<span>Zoom ({scanner.zoomValue})</span>
				<input
					class="min-h-11"
					type="range"
					min={scanner.zoom.min}
					max={scanner.zoom.max}
					step={scanner.zoom.step ?? 'any'}
					bind:value={() => scanner.zoomValue, (value) => scanner.setZoom(value)}
				/>
			</label>
		{/if}

		{#if scanner.torchAvailable}
			<button
				class={[
					'min-h-11 rounded border px-4',
					scanner.torchOn ? 'border-yellow-500 bg-yellow-300' : 'border-gray-400'
				]}
				aria-pressed={scanner.torchOn}
				onclick={scanner.toggleTorch}
			>
				Licht
			</button>
		{/if}

		<label class="flex min-h-11 items-center gap-2">
			<input
				class="size-6"
				type="checkbox"
				bind:checked={scanner.useAudioElement}
				onclick={scanner.primeAudio}
			/>
			<span>Ton über Audio-Element</span>
		</label>
		<audio src={asset('/beep.wav')} preload="auto" {@attach scanner.attachAudio}></audio>
	</section>

	<section class="rounded-xl border border-gray-300 p-4">
		<p class="min-h-10 font-mono text-4xl font-bold break-all">{scanner.lastCode}</p>
		<p>Dauer: {scanner.lastDurationMs} ms</p>
		<p>Treffer: {scanner.hitCount}</p>
	</section>

	<section>
		<h2 class="font-semibold">Letzte Codes</h2>
		<ol class="font-mono">
			{#each scanner.recent as hit (hit.id)}
				<li>{timeFormat.format(hit.at)} {hit.code}</li>
			{/each}
		</ol>
	</section>

	<section class="text-sm break-words text-gray-700">
		<h2 class="font-semibold">Status</h2>
		<p>User-Agent: {navigator.userAgent}</p>
		<p>display-mode: {standalone.current ? 'standalone' : 'browser'}</p>
		<p>Kamera: {scanner.cameraLabel}</p>
		{#each scanner.errors as error (error.id)}
			<p class="text-red-700">Fehler: {error.message}</p>
		{/each}
	</section>
</main>
