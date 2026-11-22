<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let { hint, busy, onsearch }: { hint: string; busy: boolean; onsearch: (query: string) => void } = $props();
	let query = $state('');
</script>

<form class="release-search" onsubmit={(event) => { event.preventDefault(); onsearch(query.trim()); }}>
	<label>
		<span>{t('mediafiles.releaseSheet.query')}</span>
		<input class="input" type="search" maxlength="200" placeholder={hint} bind:value={query} disabled={busy} />
	</label>
	<Button type="submit" icon="search" size="sm" disabled={busy}>{t('mediafiles.releaseSheet.searchReleases')}</Button>
</form>

<style>
	.release-search { display: flex; flex-wrap: wrap; gap: 8px; align-items: end; }
	label { display: grid; gap: 5px; flex: 1 1 180px; min-width: 0; font-size: 13px; }
	input { width: 100%; }
</style>
