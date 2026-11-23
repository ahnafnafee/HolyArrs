<script lang="ts">
	import { api, message } from '#lib/core/api.ts';
	import { bytes, date } from '#lib/core/format.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import SearchField from '#lib/discover/SearchField.svelte';
	import SearchScopes from '#lib/discover/SearchScopes.svelte';
	import { scopeAddress, type SearchScope } from '#lib/discover/search-address.ts';
	import { focusOnOpen, nsfwScope, searchField } from '#lib/core/search-field.svelte.ts';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';

	type Release = {
		id: string;
		name: string;
		source: string;
		size: number;
		seeders: number | null;
		peers: number | null;
		published?: string;
		detailsUrl?: string;
		hasThumbnail: boolean;
		canDownload: boolean;
	};
	type QueueState = { status: 'sending' | 'added' | 'duplicate' | 'pending' | 'error'; error?: string };
	type Answer = { releases: Release[]; sources: { name: string; status: 'answered' | 'paused' | 'unavailable' }[] };
	let busy = $state(false);
	let error = $state('');
	let answer = $state.raw<Answer | null>(null);
	let selected = $state.raw<Release | null>(null);
	let showDetails = $state(false);
	const missingArt = new SvelteSet<string>();
	const queued = new SvelteMap<string, QueueState>();

	function openRelease(release: Release) {
		selected = release;
		showDetails = true;
	}

	async function download(release: Release) {
		const current = queued.get(release.id);
		if (current && current.status !== 'error') return;
		queued.set(release.id, { status: 'sending' });
		try {
			const result = await api.post<{ status: 'added' | 'duplicate' | 'pending' }>(
				`/sources/nsfw-search/${release.id}/download`, {}
			);
			queued.set(release.id, result);
		} catch (e) {
			queued.set(release.id, { status: 'error', error: message(e) });
		}
	}

	function selectScope(scope: SearchScope | 'nsfw') {
		if (scope === 'nsfw') return;
		void goto(scopeAddress(new URL(to('/search'), page.url.href), scope, searchField.term));
	}

	onMount(() => {
		nsfwScope.submit = search;
		nsfwScope.exit = () => selectScope('all');
		focusOnOpen();
		return () => {
			nsfwScope.term = '';
			nsfwScope.submit = undefined;
			nsfwScope.exit = undefined;
		};
	});

	async function search() {
		const words = nsfwScope.term.trim();
		if (!words || busy || !session.can('settings') || session.kid) return;
		busy = true;
		showDetails = false;
		missingArt.clear();
		queued.clear();
		answer = null;
		error = '';
		try {
			answer = await api.post<Answer>('/sources/nsfw-search', { query: words });
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{t('discover.nsfw.title')} Ã‚Â· HolyArrs</title></svelte:head>

<section class="release-search" data-testid="nsfw-search">
	<h1 class="sr-only">{t('discover.nsfw.title')}</h1>
	{#if session.can('settings') && !session.kid}
		<div class="close-row"><Button variant="secondary" icon="x" onclick={() => selectScope('all')}>{t('discover.search.close')}</Button></div>
		<div class="phone-field"><SearchField searchKey="" testid="search-box-phone" id="q-nsfw-phone" closable={false} /></div>
		<SearchScopes value="nsfw" onchange={selectScope} />
		<div class="release-results" aria-live="polite" aria-busy={busy}>
			{#if error}
				<p class="form-error" role="alert">{error}</p>
			{:else if busy}
				<p class="muted">{t('discover.nsfw.searching')}</p>
			{:else if answer}
				{#if !answer.sources.length}
					<p class="muted">{t('discover.nsfw.noSources')}</p>
				{:else if !answer.sources.some((source) => source.status === 'answered')}
					<p class="muted">{t('discover.nsfw.noAnswers')}</p>
				{:else if !answer.releases.length}
					<p class="muted">{t('discover.nsfw.noResults')}</p>
				{:else}
					<ul class="releases">
						{#each answer.releases as release (release.id)}
							<li data-testid="nsfw-release">
								<button class="release-row" type="button" aria-haspopup="dialog" onclick={() => openRelease(release)}>
									{@render artwork(release, false)}
									<span class="release-copy">
										<strong>{release.name}</strong>
										<small class="muted">
											{release.source}{release.size > 0 ? ' Â· ' + bytes(release.size) : ''}
											{release.seeders != null ? ' Â· ' + t('mediafiles.release.seeders', { n: release.seeders }) : ''}
										</small>
									</span>
									<span class="muted"><Icon name="chevronRight" size={18} /></span>
								</button>
							</li>
						{/each}
					</ul>
				{/if}
				{#each answer.sources.filter((source) => source.status !== 'answered') as source}
					<p class="muted">{t(source.status === 'paused' ? 'discover.nsfw.sourcePaused' : 'discover.nsfw.sourceUnavailable', { name: source.name })}</p>
				{/each}
			{/if}
		</div>
	{:else}
		<p class="muted">{t('discover.nsfw.restricted')}</p>
		<div><Button href={to('/search')} variant="plain">{t('discover.nsfw.back')}</Button></div>
	{/if}
</section>

{#snippet artwork(release: Release, preview: boolean)}
	{@const url = to(`/api/v1/sources/nsfw-search/${release.id}/thumbnail`)}
	<span class="art" class:preview>
		<Icon name="image" size={preview ? 28 : 20} />
		{#if release.hasThumbnail && !missingArt.has(url)}
			<img src={url} alt="" loading={preview ? 'eager' : 'lazy'} decoding="async" onerror={() => missingArt.add(url)} />
		{/if}
	</span>
{/snippet}

{#if selected}
	{@const release = selected}
	{@const state = queued.get(release.id)}
	<Sheet bind:open={showDetails} title={release.name}>
		<div class="release-details">
			{#key release.id}
				{@render artwork(release, true)}
			{/key}
			<dl>
				<div><dt>{t('discover.nsfw.source')}</dt><dd>{release.source}</dd></div>
				{#if release.size > 0}<div><dt>{t('discover.nsfw.size')}</dt><dd>{bytes(release.size)}</dd></div>{/if}
				{#if release.published}<div><dt>{t('discover.nsfw.published')}</dt><dd>{date(release.published)}</dd></div>{/if}
				{#if release.seeders != null}<div><dt>{t('discover.nsfw.seeders')}</dt><dd>{release.seeders}</dd></div>{/if}
				{#if release.peers != null}<div><dt>{t('discover.nsfw.peers')}</dt><dd>{release.peers}</dd></div>{/if}
			</dl>
			{#if state?.status === 'error'}
				<p class="form-error" role="alert">{state.error}</p>
			{:else if state && state.status !== 'sending'}
				<p role="status">{t('discover.nsfw.' + state.status)}</p>
			{:else if !release.canDownload}
				<p class="muted">{t('discover.nsfw.noDownloader')}</p>
			{/if}
		</div>
		{#snippet footer()}
			{#if release.detailsUrl}
				<Button href={release.detailsUrl} target="_blank" rel="noopener noreferrer" iconAfter="arrowUpRight">{t('discover.nsfw.openSource')}</Button>
			{/if}
			{#if release.canDownload}
				<Button variant="primary" icon="download" busy={state?.status === 'sending'}
					disabled={!!state && state.status !== 'error'} onclick={() => download(release)}>
					{t('discover.nsfw.download')}
				</Button>
			{/if}
		{/snippet}
	</Sheet>
{/if}

<style>
	.release-search { display: flex; flex-direction: column; gap: 24px; min-width: 0; padding-bottom: 48px; }
	.release-results { width: min(100%, 800px); margin-inline: auto; }
	.close-row, .phone-field { display: none; }
	@media (max-width: 767px) {
		.close-row { display: flex; justify-content: flex-end; }
		.phone-field { position: sticky; top: 8px; z-index: 22; display: flex; justify-content: center; }
		.phone-field :global(.ui-search) { width: 100%; }
	}
	.releases { padding: 0; margin: 0; list-style: none; }
	.releases li + li { border-top: 1px solid var(--line); }
	.release-row {
		display: grid; grid-template-columns: clamp(64px, 15vw, 104px) minmax(0, 1fr) auto; align-items: center;
		gap: 12px; width: 100%; padding: 10px 8px; border: 0; border-radius: 10px;
		background: none; color: var(--text); text-align: start; cursor: pointer;
	}
	.release-row:hover { background: var(--surface-1); }
	.release-row:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.release-copy { display: grid; gap: 4px; overflow-wrap: anywhere; }
	.release-copy strong { font: var(--t-headline); }
	.release-copy small { font: var(--t-caption); }
	.art {
		display: grid; position: relative; place-items: center; aspect-ratio: 16 / 9; overflow: hidden;
		background: var(--surface-2); color: var(--text-muted); border-radius: 8px;
	}
	.art img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; background: var(--surface-2); }
	.art.preview { max-height: 320px; }
	.art.preview img { object-fit: contain; }
	.release-details { display: grid; gap: 16px; }
	.release-details dl { display: flex; flex-wrap: wrap; gap: 16px 24px; margin: 0; }
	.release-details dt { font: var(--t-caption); color: var(--text-muted); }
	.release-details dd { margin: 3px 0 0; font: var(--t-headline); }
	.release-details p { margin: 0; }
</style>
