// Drives the search box at the top of the home page. Nothing is matched here:
// every keystroke asks the Go endpoint at /search, which answers with the rows
// to list and the artists to keep on screen. This file only decides when to ask,
// draws the answer, and moves the highlight around.
//
// Who owns the grid:
//
//   - While the box is empty the filter sidebar drives the grid, exactly as it
//     did before the search existed.
//   - From the first character on the search takes over: the sidebar is switched
//     off and dimmed, and only this file writes to a card.
//   - Emptying the box hands the grid straight back.
//
// The handover goes through window.groupie_filters, which static/js/filters.js
// publishes. That way the two scripts share one agreement instead of both
// reaching for the same cards and hoping.

const search_box = document.getElementById("search_box");
const search_input = document.getElementById("search_input");
const search_suggestions_list = document.getElementById("search_suggestions");

// The grid, the count line and the empty state are the same three elements the
// filters use. They are looked up again under their own names here because both
// scripts share one global scope and a second const of the same name would be a
// redeclaration.
const searched_artist_grid = document.getElementById("artist_grid");
const searched_result_count = document.getElementById("result_count");
const searched_empty_state = document.getElementById("empty_state");

// How long the typing has to pause before the request goes out. Short enough to
// feel immediate, long enough that typing a word is one request and not six.
const debounce_delay_ms = 150;

// The artist rows are the ones that carry the band underneath the label, since
// an artist row would only be repeating itself.
const artist_suggestion_type = "artist/band";

let searched_artist_cards = [];
let suggestion_rows = [];
let active_suggestion_index = -1;
let debounce_timer_id = 0;
let latest_request_number = 0;
let search_owns_the_grid = false;


/* --------------------------------------------------------------------------
   The dropdown
   -------------------------------------------------------------------------- */

// close_suggestions folds the list away and forgets which row was highlighted.
// The grid is left exactly as it is, because the box may still hold a query.
const close_suggestions = () => {
    suggestion_rows = [];
    active_suggestion_index = -1;

    search_suggestions_list.textContent = "";
    search_suggestions_list.hidden = true;
    search_input.setAttribute("aria-expanded", "false");
    search_input.removeAttribute("aria-activedescendant");
};

// highlight_suggestion moves the highlight to one row, or clears it altogether
// when given -1.
const highlight_suggestion = (suggestion_index) => {
    suggestion_rows.forEach((suggestion_row, row_index) => {
        const row_is_active = row_index === suggestion_index;

        suggestion_row.classList.toggle("is-active", row_is_active);
        suggestion_row.setAttribute("aria-selected", String(row_is_active));
    });

    active_suggestion_index = suggestion_index;

    if (suggestion_index < 0) {
        search_input.removeAttribute("aria-activedescendant");
        return;
    }

    // Naming the highlighted row on the input is what lets a screen reader
    // follow the arrow keys, since the focus itself never leaves the box.
    search_input.setAttribute("aria-activedescendant", suggestion_rows[suggestion_index].id);

    // Keeps the highlighted row in view once the list is taller than its panel.
    suggestion_rows[suggestion_index].scrollIntoView({ block: "nearest" });
};

// move_active_suggestion walks the highlight up or down, wrapping at both ends
// so that holding an arrow key goes round the list rather than sticking.
const move_active_suggestion = (step) => {
    if (suggestion_rows.length === 0) {
        return;
    }

    const row_count = suggestion_rows.length;
    let next_index = 0;

    if (active_suggestion_index < 0) {
        // Nothing highlighted yet: down starts at the top, up at the bottom.
        next_index = step > 0 ? 0 : row_count - 1;
    } else {
        next_index = (active_suggestion_index + step + row_count) % row_count;
    }

    highlight_suggestion(next_index);
};

// open_suggestion goes to the page of the artist a row belongs to.
const open_suggestion = (suggestion_index) => {
    const suggestion_row = suggestion_rows[suggestion_index];
    if (suggestion_row === undefined) {
        return;
    }

    window.location.href = "/artist?id=" + suggestion_row.dataset.artistId;
};

// build_suggestion_row draws one row: what matched, what kind of thing it is,
// and, for anything that is not an artist, which artist it belongs to. That last
// line is what tells apart the two rows the subject's own example produces.
const build_suggestion_row = (suggestion, suggestion_index) => {
    const suggestion_row = document.createElement("li");
    suggestion_row.className = "search-suggestion";
    suggestion_row.id = "search_suggestion_" + suggestion_index;
    suggestion_row.setAttribute("role", "option");
    suggestion_row.setAttribute("aria-selected", "false");
    suggestion_row.dataset.artistId = String(suggestion.artistID);

    const label_element = document.createElement("span");
    label_element.className = "suggestion-label";
    // Written as text rather than as markup, so a name holding a character such
    // as < is shown and never interpreted.
    label_element.textContent = suggestion.label;

    const type_element = document.createElement("span");
    type_element.className = "suggestion-type";
    type_element.textContent = suggestion.type;

    suggestion_row.appendChild(label_element);
    suggestion_row.appendChild(type_element);

    if (suggestion.type !== artist_suggestion_type) {
        const artist_element = document.createElement("span");
        artist_element.className = "suggestion-artist";
        artist_element.textContent = suggestion.artistName;

        suggestion_row.appendChild(artist_element);
    }

    // mousedown rather than click: the box losing focus would otherwise close
    // the list out from under the pointer before the click ever landed.
    suggestion_row.addEventListener("mousedown", (mouse_event) => {
        mouse_event.preventDefault();
        open_suggestion(suggestion_index);
    });

    suggestion_row.addEventListener("mouseenter", () => {
        highlight_suggestion(suggestion_index);
    });

    return suggestion_row;
};

// render_suggestions replaces the whole list with the answer that just arrived.
const render_suggestions = (suggestions) => {
    search_suggestions_list.textContent = "";
    suggestion_rows = [];
    active_suggestion_index = -1;
    search_input.removeAttribute("aria-activedescendant");

    if (suggestions.length === 0) {
        // A query that matches nothing still gets a line of its own, so that an
        // empty list is never mistaken for the search having stopped working.
        const empty_row = document.createElement("li");
        empty_row.className = "search-suggestion search-suggestion-empty";
        empty_row.textContent = "No matches";

        search_suggestions_list.appendChild(empty_row);
    } else {
        suggestions.forEach((suggestion, suggestion_index) => {
            const suggestion_row = build_suggestion_row(suggestion, suggestion_index);

            suggestion_rows.push(suggestion_row);
            search_suggestions_list.appendChild(suggestion_row);
        });
    }

    search_suggestions_list.hidden = false;
    search_input.setAttribute("aria-expanded", "true");
};


/* --------------------------------------------------------------------------
   The grid, while the search owns it
   -------------------------------------------------------------------------- */

// apply_search_to_grid keeps only the artists the query reached. The IDs come
// from the endpoint and cover the whole match, not just the handful of rows the
// dropdown had room for.
const apply_search_to_grid = (matched_artist_ids) => {
    // Compared as text, because a data attribute is always text.
    const visible_artist_ids = new Set(matched_artist_ids.map(String));

    let matching_card_count = 0;
    searched_artist_cards.forEach((card_element) => {
        const card_matches = visible_artist_ids.has(card_element.dataset.artistId);

        card_element.hidden = !card_matches;
        if (card_matches) {
            matching_card_count += 1;
        }
    });

    // The same count line and the same empty state the filters use, worded the
    // same way, so the page reads identically whichever of the two is in charge.
    searched_result_count.textContent =
        matching_card_count + " of " + searched_artist_cards.length + " artists";

    searched_empty_state.hidden = matching_card_count > 0;
    searched_artist_grid.hidden = matching_card_count === 0;
};

// take_grid_from_filters switches the sidebar off, the first time the box goes
// from empty to holding something.
const take_grid_from_filters = () => {
    if (search_owns_the_grid) {
        return;
    }

    search_owns_the_grid = true;
    window.groupie_filters.set_enabled(false);
};

// release_grid_to_filters gives the grid back once the box is empty again, and
// lets the sidebar redraw it from whatever its controls still say.
const release_grid_to_filters = () => {
    if (!search_owns_the_grid) {
        return;
    }

    search_owns_the_grid = false;
    window.groupie_filters.set_enabled(true);
    window.groupie_filters.apply();
};


/* --------------------------------------------------------------------------
   Asking the server
   -------------------------------------------------------------------------- */

// request_suggestions fetches one query's answer and draws it.
const request_suggestions = (raw_query) => {
    // Every request is numbered and only the newest one's answer is used. Two
    // keystrokes in quick succession can come back out of order, and the older
    // answer landing last would leave the page showing the wrong query.
    latest_request_number += 1;
    const this_request_number = latest_request_number;

    fetch("/search?q=" + encodeURIComponent(raw_query))
        .then((response) => response.json())
        .then((search_response) => {
            if (this_request_number !== latest_request_number) {
                return;
            }

            render_suggestions(search_response.suggestions);
            apply_search_to_grid(search_response.artistIDs);
        })
        .catch(() => {
            if (this_request_number !== latest_request_number) {
                return;
            }

            // A request that failed is not the same as a query that matched
            // nothing, so the list is folded away rather than being filled with
            // a wrong answer.
            close_suggestions();
        });
};

// handle_search_input runs on every keystroke, and on the little clear cross the
// browser draws inside a search input.
const handle_search_input = () => {
    const raw_query = search_input.value.trim();

    window.clearTimeout(debounce_timer_id);

    if (raw_query === "") {
        // Nothing to search on, so the page is left exactly as it is with an
        // empty box: no list, and the sidebar back in charge of the grid.
        //
        // Answered at once instead of after the delay, so the grid comes back
        // the instant the last character is deleted. Bumping the request number
        // makes any answer still in flight stale, so it cannot arrive
        // afterwards and narrow the grid again.
        latest_request_number += 1;

        close_suggestions();
        release_grid_to_filters();
        return;
    }

    // Taken over straight away rather than when the answer arrives, so the
    // sidebar cannot be touched during the wait.
    take_grid_from_filters();

    debounce_timer_id = window.setTimeout(() => {
        request_suggestions(raw_query);
    }, debounce_delay_ms);
};

// handle_search_keydown works the list from the keyboard, so the search can be
// used without ever reaching for the mouse.
const handle_search_keydown = (key_event) => {
    if (key_event.key === "ArrowDown") {
        // Stops the caret jumping to the end of the text as well.
        key_event.preventDefault();
        move_active_suggestion(1);
        return;
    }

    if (key_event.key === "ArrowUp") {
        key_event.preventDefault();
        move_active_suggestion(-1);
        return;
    }

    if (key_event.key === "Enter") {
        // Only a highlighted row opens an artist. Pressing Enter with nothing
        // highlighted leaves the narrowed grid alone, which is already an answer
        // in itself.
        if (active_suggestion_index >= 0) {
            key_event.preventDefault();
            open_suggestion(active_suggestion_index);
        }
        return;
    }

    if (key_event.key === "Escape") {
        close_suggestions();
    }
};


/* --------------------------------------------------------------------------
   Start up
   -------------------------------------------------------------------------- */

const initialise_search = () => {
    // The script belongs to the home page, and it needs the hook filters.js
    // publishes, since the two take turns driving the grid. Bailing out rather
    // than throwing keeps it harmless anywhere else.
    if (search_input === null ||
        searched_artist_grid === null ||
        window.groupie_filters === undefined) {
        return;
    }

    searched_artist_cards = Array.from(searched_artist_grid.querySelectorAll(".card"));

    search_input.addEventListener("input", handle_search_input);
    search_input.addEventListener("keydown", handle_search_keydown);

    // A click anywhere else folds the list away. The grid stays as the search
    // left it, because the box still holds the query.
    document.addEventListener("click", (click_event) => {
        if (!search_box.contains(click_event.target)) {
            close_suggestions();
        }
    });

    // A browser restoring the typed text across a reload does so without firing
    // anything, so one run at the start puts the page back in step with whatever
    // is in the box.
    if (search_input.value.trim() !== "") {
        handle_search_input();
    }
};

initialise_search();
