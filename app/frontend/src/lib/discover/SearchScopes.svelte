<script module lang="ts">
	import type { IconName } from '#lib/ui/Icon.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { partOn } from '#lib/mediafiles/index.ts';
	import type { SearchScope } from './search-address.ts';

	type Scope = SearchScope | 'nsfw';
	type ScopeOption = { value: Scope; label: string; icon: IconName };

	export function standardSearchScopes(): (ScopeOption & { value: SearchScope })[] {
		return ([
			{ value: 'all', label: t('discover.search.all'), icon: 'grid' },
			{ value: 'people', label: t('discover.search.people'), icon: 'people' },
			{ value: 'movie', label: t('discover.search.movies'), icon: 'film' },
			{ value: 'show', label: t('discover.search.tvShows'), icon: 'tv' },
			{ value: 'music', label: t('discover.search.music'), icon: 'music' }
		] satisfies (ScopeOption & { value: SearchScope })[]).filter(
			(o) => o.value === 'show' ? partOn('shows') : o.value === 'movie' ? partOn('movies') : o.value === 'music' ? partOn('music') : true
		);
	}
</script>

<script lang="ts">
	import { session } from '#lib/core/session.svelte.ts';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';

	let { value, onchange }: { value: Scope; onchange: (scope: Scope) => void } = $props();
	const allowed = $derived(session.can('settings') && !session.kid);
	const standard = $derived(standardSearchScopes());
	const options = $derived<ScopeOption[]>(allowed
		? [...standard, { value: 'nsfw', label: t('discover.nsfw.tab'), icon: 'shield' }]
		: standard);
</script>

{#if standard.length > 2 || allowed}
	<div class="types" data-pin-pills>
		<SegmentedControl {options} {value} label={t('discover.search.lookIn')} testid="search-scope" {onchange} />
	</div>
{/if}

<style>
	.types { display: flex; justify-content: center; }
	.types :global(.ui-seg) { background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	@media (max-width: 767px) { .types :global(.ui-seg) { width: 100%; grid-auto-columns: minmax(0, 1fr); } .types :global(.ui-seg button) { padding-inline: 6px; min-width: 0; } }
	@media (max-width: 520px) { .types :global(.ui-seg button) { padding-inline: 2px; font-size: 12.5px; } .types :global(.ui-seg button svg) { display: none; } }
</style>
