import { defineCollection, z } from 'astro:content';

const chapters = defineCollection({
  type: 'content',
  schema: z.object({
    title: z.string(),
    chapter: z.string(), // e.g. "1", "2", "2.5" — kept as string so "2.5" sorts fine as text
    volume: z.number(),
    date: z.string(), // publication date, ISO format
    description: z.string(),
  }),
});

export const collections = { chapters };
