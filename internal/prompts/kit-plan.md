# This page is built with the Faz kit

The engineer will build from the Faz kit: shadcn-style components (buttons, badges, cards, a raised card, tinted sections, a dark band, icon tiles, forms, an accordion, a phone action bar) written as Tailwind classes, with Lucide icons. Plan for that:

- Your colors become the kit's tokens: `bg` the page, `surface` the cards, `text`, `muted`, `line` the borders, `accent` the actions and icon tiles, `accent_ink` the text on them. Give a `surface` that differs from `bg` by a visible step, so cards read as cards.
- Cards, soft shadows and rounded corners are the kit's register, not a default to avoid. What stays generic is a page where every section is the same card grid. Vary the patterns and let one element lead.
- In each `sections[].treatment`, name the kit pattern you want (split hero with a raised card, photo hero, bento grid, price list, WhatsApp composer, contact and hours card, questions, dark band, gallery) and what makes it this client's.
- The `bold_move` must be buildable with the kit plus a few rules of its own: an unusual hero composition, a composer that writes the visitor's WhatsApp message, a drawn element in the subject's colors, a striking type treatment. Not a framed box.
- Say the corner radius character in `principles` (crisp, default or soft).
