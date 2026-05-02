# Product

## Register

product

## Users

Emerging digital artists showcasing portfolios, and viewers browsing for visual inspiration. Artists upload work expecting it to look gallery-grade — comparable to an ArtStation portfolio — and judge platform credibility on presentation quality. Viewers scroll long feeds on desktop and mobile, often at night, expecting dense visual signal with minimal interface friction.

## Product Purpose

art-web is an image-sharing app where the **artwork is the product** and the interface is the frame around it. Success is measured by whether an artist's work looks *better* here than on a default gallery template, and whether viewers can browse a long feed without the chrome ever pulling attention away from the images. The non-gallery surfaces (upload, settings, auth) are pure tools — they should disappear into the task.

## Brand Personality

Cinematic, technical, reverent of the work. Speaks like a print magazine art editor who also writes shader code: confident in its taste, precise in its language, never decorative for its own sake.

Three words: **gallery-grade, image-forward, ink-dark**.

## Anti-references

- Behance / Dribbble cream-and-rounded-cards default
- Pinterest pastel masonry
- Instagram square-feed UI
- SaaS-blue dashboard chrome (Linear blue, Notion gray)
- AI-tool neon-on-black cliché (Midjourney website, Stable Diffusion UIs)
- Glassmorphism, gradient text, side-stripe borders, hero-metric grids
- Warm museum / wine-bar / art-fair-brochure aesthetics
- The previous warm-sand iteration of this project — full reset

## Design Principles

1. **The artwork is the only protagonist.** Chrome contains, never decorates. If a UI element competes with an image, the UI is wrong.
2. **Density is reverence.** Tightly packed feeds at gallery resolution communicate "there is real work here," not "we're padding for elegance." Whitespace serves rhythm, not breathing room.
3. **Reveal on demand.** Title, artist, tags, dimensions appear when wanted (hover, focus, detail view) and disappear otherwise. The feed reads as imagery first, library second.
4. **Editorial confidence over decoration.** One sans family, one mono for technical metadata. Weight contrast carries hierarchy; no serif swapped in for "art feeling".
5. **The dark is intentional.** Near-black canvas chosen because galleries dim the room around the work. The chrome is the dim room; the artwork is the spotlight.

## Accessibility & Inclusion

- WCAG AA contrast for all text and interactive controls. Hover-revealed metadata must reach AA against the gradient scrim it sits on.
- Respect `prefers-reduced-motion` — image hover lift, scrim fade, and feed entrance transitions all opt out.
- Keyboard navigation must reach every artwork tile and reveal the same metadata on focus that hover surfaces.
- Forms (upload, settings) follow standard product conventions; familiarity is an accessibility feature there.
