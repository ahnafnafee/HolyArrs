// Hidden from browsing: owners and admins hide a title or a genre from Explore, New & Popular, search and suggestions, for everyone or one person.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { Hide } from '#lib/core/types.ts';
import { SvelteMap } from 'svelte/reactivity';

export type { Hide };

export type HideAsk = { kind: 'movie' | 'show'; tmdb_id?: string; genre?: string; name?: string; user_id?: number };

const quickHides = new SvelteMap<string, number>();
const quickKey = (userId: number, kind: string, tmdb: string) => `${userId}:${kind}:${tmdb}`;

export function hiddenInCurrentView(userId: number | undefined, title: { kind: string; tmdb: string } | null | undefined): boolean {
	return !!userId && !!title && quickHides.has(quickKey(userId, title.kind, title.tmdb));
}

type Client = {
	get<T>(p: string): Promise<T>;
	post<T>(p: string, body?: unknown): Promise<T>;
	del<T = void>(p: string): Promise<T>;
	freshBrowsing(): void;
};

export const loadHides = (api: Client, userId?: number) => api.get<{ hidden: Hide[] }>(`/hidden${userId === undefined ? '' : `?user_id=${userId}`}`).then((r) => r.hidden ?? []);

export async function addHide(api: Client, ask: HideAsk): Promise<Hide> {
	const h = await api.post<Hide>('/hidden', ask);
	api.freshBrowsing();
	return h;
}

export async function quickHide(api: Client, userId: number, kind: HideAsk['kind'], tmdb: string, name: string): Promise<Hide> {
	const h = await addHide(api, { kind, tmdb_id: tmdb, name, user_id: userId });
	quickHides.set(quickKey(userId, kind, tmdb), h.id);
	return h;
}

export async function showAgain(api: Client, id: number): Promise<void> {
	await api.del(`/hidden/${id}`);
	api.freshBrowsing();
	for (const [key, hideId] of quickHides) {
		if (hideId === id) quickHides.delete(key);
	}
}

/** The hide's name: the title, or the genre. */
export const hideName = (h: Hide) => h.genre ?? h.name ?? `TMDB ${h.tmdb_id ?? ''}`.trim();

/** Who a hide is for: Everyone, or the person's name. */
export function hideFor(h: Pick<Hide, 'user_id'>, people: ReadonlyMap<number, string>): string {
	if (!h.user_id) return t('discover.hidden.everyone');
	return people.get(h.user_id) ?? t('discover.hidden.someone');
}

/** The grey line under a hide: "Movie · Everyone", "Show genre · Sam". */
export function hideLine(h: Hide, people: ReadonlyMap<number, string>): string {
	const what = h.genre ? (h.kind === 'show' ? t('discover.hidden.showGenre') : t('discover.hidden.movieGenre')) : h.kind === 'show' ? t('discover.hidden.show') : t('discover.hidden.movie');
	return `${what} · ${hideFor(h, people)}`;
}

/** The title page pill: "Hidden from browsing · Everyone", or with the count when more than one hide covers it. */
export function hiddenPill(hides: Hide[], people: ReadonlyMap<number, string>): string {
	const base = t('discover.hidden.pill');
	if (hides.length === 0) return base;
	// A hide for everyone covers each person; people count by id so names still loading never merge.
	if (hides.some((h) => !h.user_id)) return `${base} · ${t('discover.hidden.everyone')}`;
	const ids = [...new Set(hides.map((h) => h.user_id))];
	return `${base} · ${ids.length === 1 ? hideFor(hides[0], people) : t('discover.hidden.peopleCount', { n: ids.length })}`;
}

