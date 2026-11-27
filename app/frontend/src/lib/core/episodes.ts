// Episode label like S01E02.
export function episodeCode(season: number, episode: number): string {
	const two = (n: number) => String(n).padStart(2, '0');
	return `S${two(season)}E${two(episode)}`;
}

/** Uses the primary slot unless an unmonitored folder already holds the episode. */
export function availableEpisodeSlot<T extends { state: string; file_id?: number; file?: unknown; other_folder?: boolean }>(slots: T[]): T | undefined {
	const primary = slots[0];
	if (!primary || primary.file_id || primary.file) return primary;
	return slots.find((slot) => slot.other_folder && (slot.file_id || slot.file) &&
		(slot.state === 'in_library' || slot.state === 'better_wanted')) ?? primary;
}

/** An episode label with its name for a second line: "S01E02 · Flood", or the label alone while the name is unknown. "TBA" names are left out. */
export function episodeNamed(label: string, name?: string | null): string {
	const n = name?.trim();
	return n && n.toUpperCase() !== 'TBA' ? `${label} · ${n}` : label;
}

/** episodeNamed for a season and episode number ("E02" when the season is unknown). */
export function episodeWords(season: number | undefined | null, episode: number, name?: string | null): string {
	return episodeNamed(season == null ? `E${String(episode).padStart(2, '0')}` : episodeCode(season, episode), name);
}
