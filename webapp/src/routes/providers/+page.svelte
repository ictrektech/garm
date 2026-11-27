<script lang="ts">
	import { onMount } from 'svelte';
	import { garmApi, type Provider, type ProviderRunnerConfig } from '$lib/api/client.js';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Button from '$lib/components/Button.svelte';
	import { toastStore } from '$lib/stores/toast.js';
	import { isCurrentUserAdmin } from '$lib/utils/jwt';

	// One card per provider. Only external providers that speak the V-Scheduler
	// config format get a form; the rest state why they have none.
	interface ProviderState {
		provider: Provider;
		config: ProviderRunnerConfig | null;
		baseUrl: string;
		imageRepository: string;
		imageTag: string;
		token: string;
		loading: boolean;
		saving: boolean;
		error: string;
	}

	let states: ProviderState[] = [];
	let loading = true;
	let error = '';
	const admin = isCurrentUserAdmin();

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			const providers = await garmApi.listProviders();
			states = await Promise.all(providers.map(loadProvider));
		} catch (err) {
			error = message(err);
		} finally {
			loading = false;
		}
	}

	async function loadProvider(provider: Provider): Promise<ProviderState> {
		const state: ProviderState = {
			provider,
			config: null,
			baseUrl: '',
			imageRepository: '',
			imageTag: '',
			token: '',
			loading: provider.type === 'external',
			saving: false,
			error: ''
		};
		if (provider.type !== 'external') {
			return state;
		}
		try {
			const config = await garmApi.getProviderRunnerConfig(provider.name ?? '');
			state.config = config;
			state.baseUrl = config.base_url ?? '';
			state.imageRepository = config.image_repository ?? '';
			state.imageTag = config.image_tag ?? '';
		} catch (err) {
			state.error = message(err);
		} finally {
			state.loading = false;
		}
		return state;
	}

	async function save(index: number) {
		const name = states[index].provider.name ?? '';
		patch(index, { saving: true, error: '' });
		try {
			const config = await garmApi.updateProviderRunnerConfig(name, {
				base_url: states[index].baseUrl.trim(),
				token: states[index].token,
				image_repository: states[index].imageRepository.trim(),
				image_tag: states[index].imageTag.trim()
			});
			patch(index, { config, token: '' });
			toastStore.success('Saved', `${name} runner settings updated`);
		} catch (err) {
			const text = message(err);
			patch(index, { error: text });
			toastStore.error('Save failed', text);
		} finally {
			patch(index, { saving: false });
		}
	}

	// Replacing the item is what makes the card re-render. Mutating a field of
	// an each-block item on its own does not schedule an update.
	function patch(index: number, changes: Partial<ProviderState>) {
		states[index] = { ...states[index], ...changes };
	}

	function message(err: unknown): string {
		return err instanceof Error ? err.message : String(err);
	}
</script>

<PageHeader
	title="Providers"
	description="Runner settings that garm passes to external providers. The service token is stored on the server and never shown again."
/>

{#if !admin}
	<div class="mt-6 rounded-lg border border-yellow-200 bg-yellow-50 p-4 text-sm text-yellow-800 dark:border-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-200">
		Only administrators can read or change these settings.
	</div>
{/if}

{#if loading}
	<div class="mt-6 space-y-4">
		{#each [1, 2] as _}
			<div class="h-32 animate-pulse rounded-lg bg-gray-200 dark:bg-gray-700"></div>
		{/each}
	</div>
{:else if error}
	<div role="alert" class="mt-6 rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-800 dark:border-red-700 dark:bg-red-900/30 dark:text-red-200">
		{error}
	</div>
{:else}
	<div class="mt-6 space-y-6">
		{#each states as state, index (state.provider.name)}
			<section class="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
				<div class="flex flex-wrap items-center justify-between gap-3">
					<div>
						<h2 class="text-lg font-semibold text-gray-900 dark:text-white">{state.provider.name}</h2>
						<p class="text-sm text-gray-500 dark:text-gray-400">{state.provider.type}</p>
					</div>
					{#if state.config?.supported}
						<span
							class="rounded-full px-3 py-1 text-xs font-medium
								{state.config.token_configured
									? 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-200'
									: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-200'}"
						>
							{state.config.token_configured ? 'Configured' : 'No token'}
						</span>
					{/if}
				</div>

				{#if state.loading}
					<p class="mt-4 text-sm text-gray-500 dark:text-gray-400">Loading…</p>
				{:else if state.error && !state.config}
					<p role="alert" class="mt-4 text-sm text-red-600 dark:text-red-400">{state.error}</p>
				{:else if !state.config?.supported}
					<p class="mt-4 text-sm text-gray-500 dark:text-gray-400">
						This provider does not use garm runner settings, so there is nothing to configure.
					</p>
				{:else}
					{#if state.error}
						<p role="alert" class="mt-4 text-sm text-red-600 dark:text-red-400">{state.error}</p>
					{/if}
					<div class="mt-4 grid gap-4 sm:grid-cols-2">
						<label class="block text-sm">
							<span class="font-medium text-gray-700 dark:text-gray-300">Scheduler base URL</span>
							<input
								type="text"
								bind:value={state.baseUrl}
								placeholder="http://v-scheduler:38120/api/v1"
								class="mt-1 w-full rounded-md border border-gray-300 px-3 py-2 text-gray-900 shadow-sm focus:border-blue-500 focus:outline-none focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
							/>
						</label>
						<label class="block text-sm">
							<span class="font-medium text-gray-700 dark:text-gray-300">Service token</span>
							<input
								type="password"
								bind:value={state.token}
								autocomplete="new-password"
								placeholder={state.config.token_configured ? 'Stored. Leave empty to keep it.' : 'Paste the token from V-Scheduler'}
								class="mt-1 w-full rounded-md border border-gray-300 px-3 py-2 text-gray-900 shadow-sm focus:border-blue-500 focus:outline-none focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
							/>
						</label>
						<label class="block text-sm">
							<span class="font-medium text-gray-700 dark:text-gray-300">Runner image repository</span>
							<input
								type="text"
								bind:value={state.imageRepository}
								placeholder="swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm-runner"
								class="mt-1 w-full rounded-md border border-gray-300 px-3 py-2 text-gray-900 shadow-sm focus:border-blue-500 focus:outline-none focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
							/>
						</label>
						<label class="block text-sm">
							<span class="font-medium text-gray-700 dark:text-gray-300">Runner image tag</span>
							<input
								type="text"
								bind:value={state.imageTag}
								placeholder="v6.0.1_27ec9940"
								class="mt-1 w-full rounded-md border border-gray-300 px-3 py-2 text-gray-900 shadow-sm focus:border-blue-500 focus:outline-none focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
							/>
						</label>
					</div>
					<p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
						The tag has no amd_ or arm_ prefix. The provider adds it from the pool architecture.
					</p>
					<div class="mt-4 flex justify-end">
						<Button variant="primary" disabled={!admin || state.saving} on:click={() => save(index)}>
							{state.saving ? 'Saving…' : 'Save'}
						</Button>
					</div>
				{/if}
			</section>
		{:else}
			<p class="text-sm text-gray-500 dark:text-gray-400">No providers are configured.</p>
		{/each}
	</div>
{/if}

<div class="mt-8 rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm text-gray-600 dark:border-gray-700 dark:bg-gray-800/60 dark:text-gray-300">
	<p class="font-medium text-gray-800 dark:text-gray-200">Where the token comes from</p>
	<ol class="mt-2 list-decimal space-y-1 pl-5">
		<li>In V-Scheduler, open Node management → Service credentials.</li>
		<li>Create a credential for garm and copy it. It is shown once.</li>
		<li>Grant targets and repositories to garm. The pool fails with 403 until you do.</li>
		<li>Paste the token here and save.</li>
	</ol>
</div>
