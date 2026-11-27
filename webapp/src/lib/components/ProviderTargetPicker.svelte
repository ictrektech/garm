<script lang="ts">
	import { garmApi, type ProviderTarget } from '$lib/api/client.js';

	// Provider name comes from the surrounding form. The picker only renders when
	// the provider publishes a target list, so a plain provider keeps the old
	// free-form extra specs editor.
	export let provider: string;
	export let extraSpecs: string;
	export let osArch: string = 'amd64';

	let targets: ProviderTarget[] = [];
	let loading = false;
	let loadedFor = '';

	$: if (!provider) {
		targets = [];
		loadedFor = '';
	} else if (provider !== loadedFor) {
		loadedFor = provider;
		void load(provider);
	}

	async function load(name: string) {
		loading = true;
		targets = [];
		try {
			const result = await garmApi.listProviderTargets(name);
			if (name === loadedFor) {
				targets = result;
			}
		} catch (error) {
			console.warn('cannot list provider targets', error);
			targets = [];
		} finally {
			loading = false;
		}
	}

	$: selection = selectionOf(extraSpecs);
	$: profiles = profileGroups(targets);
	$: options = new Set([
		...profiles.map((group) => `profile:${group.profile}`),
		...targets.map((node) => `node:${node.id}`)
	]);
	$: staleSelection = selection !== '' && !options.has(selection);

	function selectionOf(specs: string): string {
		try {
			const parsed = JSON.parse(specs || '{}') ?? {};
			if (typeof parsed.target_id === 'string' && parsed.target_id) {
				return `node:${parsed.target_id}`;
			}
			if (typeof parsed.profile === 'string' && parsed.profile) {
				return `profile:${parsed.profile}`;
			}
		} catch {
			// Mid-edit JSON. Keep the picker on "not specified".
		}
		return '';
	}

	function profileGroups(nodes: ProviderTarget[]) {
		const counts = new Map<string, number>();
		for (const node of nodes) {
			if (!node.profile || node.status !== 'online') continue;
			counts.set(node.profile, (counts.get(node.profile) ?? 0) + 1);
		}
		return [...counts.entries()]
			.map(([profile, online]) => ({ profile, online }))
			.sort((a, b) => a.profile.localeCompare(b.profile));
	}

	function staleLabel(value: string): string {
		if (value.startsWith('node:')) {
			return `node ${value.slice('node:'.length)}`;
		}
		return `profile ${value.slice('profile:'.length)}`;
	}

	function archOfTarget(node: ProviderTarget | undefined): string | undefined {
		switch ((node?.arch ?? '').toLowerCase()) {
			case 'aarch64':
			case 'arm64':
				return 'arm64';
			case 'x86_64':
			case 'amd64':
				return 'amd64';
			default:
				return undefined;
		}
	}

	function apply(value: string) {
		let specs: Record<string, unknown> = {};
		try {
			const parsed = JSON.parse(extraSpecs || '{}');
			if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
				specs = parsed as Record<string, unknown>;
			}
		} catch {
			// Keep what the user typed. Rewriting it would drop their edits without a word.
			return;
		}

		delete specs.profile;
		delete specs.target_id;

		if (value.startsWith('profile:')) {
			specs.profile = value.slice('profile:'.length);
		} else if (value.startsWith('node:')) {
			const node = targets.find((item) => item.id === value.slice('node:'.length));
			if (node?.profile) {
				specs.profile = node.profile;
			}
			specs.target_id = node?.id ?? value.slice('node:'.length);
			const arch = archOfTarget(node);
			if (arch) {
				osArch = arch;
			}
		}

		extraSpecs = JSON.stringify(specs, null, 2);
	}
</script>

{#if targets.length > 0}
	<div>
		<label
			for="providerTarget"
			title="A profile lets the scheduler pick a node by load. A specific node pins the pool to one machine. Either way the node has to be granted to this app in V-Scheduler, otherwise creating an instance fails with 403."
			class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
		>
			Deployment Target
		</label>
		<select
			id="providerTarget"
			value={selection}
			on:change={(event) => apply(event.currentTarget.value)}
			class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
		>
			<option value="">Not specified (edit extra specs below)</option>
			{#if staleSelection}
				<option value={selection} disabled>{staleLabel(selection)} (not in the current list)</option>
			{/if}
			{#if profiles.length > 0}
				<optgroup label="Profile (node picked by the scheduler)">
					{#each profiles as group}
						<option value={`profile:${group.profile}`}>
							{group.profile} ({group.online} online)
						</option>
					{/each}
				</optgroup>
			{/if}
			<optgroup label="Specific node (pinned)">
				{#each targets as node}
					<option value={`node:${node.id}`} disabled={node.status !== 'online'}>
						{node.name || node.id} ({node.profile || 'no profile'}, {node.arch || 'unknown arch'},
						{node.status || 'unknown'})
					</option>
				{/each}
			</optgroup>
		</select>
		<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
			Profile picks by load. Node pins. Both need a V-Scheduler grant.
		</p>
	</div>
{:else if loading}
	<div class="animate-pulse bg-gray-200 dark:bg-gray-700 h-10 rounded"></div>
{/if}
