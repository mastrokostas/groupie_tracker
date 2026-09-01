// Drives the filter sidebar on the home page. Every artist card is rendered by
// the server with the values it can be filtered on already sitting in its data
// attributes, so this file only has to read the controls, decide card by card
// whether the card still matches, and hide the ones that do not. Nothing is
// fetched and the page never reloads.
//
// How the four filters combine:
//
//   - The four categories are combined with AND, so a card has to pass all four
//     of them to stay on screen.
//   - Inside the members and the locations categories the ticked values are
//     combined with OR, and a category with nothing ticked does not narrow
//     anything down at all.
//   - The locations filter looks only at the ticked place boxes. A country box
//     is a bulk switch that ticks every place underneath it, which is what makes
//     ticking one country match every city in that country.

const artist_grid = document.getElementById("artist_grid");
const result_count_element = document.getElementById("result_count");
const empty_state_element = document.getElementById("empty_state");
const filters_sidebar = document.getElementById("filters_sidebar");
const filters_reset_button = document.getElementById("filters_reset");
const filters_toggle_button = document.getElementById("filters_toggle");
const search_input_element = document.getElementById("search_input");
const page_header_element = document.getElementById("page_header");

// The cards are never added or removed after the server rendered them, so the
// list is collected once at start up rather than on every drag and click.
let artist_cards = [];

// The search bar takes the grid over while it has text in it. Only one of the
// two may write to a card, so this flag says which of them is in charge, and
// this file keeps its hands off the grid entirely while it is false.
let filters_drive_the_grid = true;


/* --------------------------------------------------------------------------
   Reading the controls and matching the cards against them
   -------------------------------------------------------------------------- */

// collect_checked_values returns the values of every ticked box matching the
// given selector, as a set so that looking a value up is immediate.
const collect_checked_values = (css_selector) => {
    const checked_values = new Set();

    document.querySelectorAll(css_selector).forEach((checkbox) => {
        if (checkbox.checked) {
            checked_values.add(checkbox.value);
        }
    });

    return checked_values;
};

// read_active_filters takes one snapshot of the whole sidebar, so that every
// card is measured against the same state.
const read_active_filters = () => {
    const creation_slider = document.querySelector('.range-slider[data-range-name="creation"]');
    const album_slider = document.querySelector('.range-slider[data-range-name="album"]');

    return {
        creation_minimum: Number(creation_slider.querySelector(".range-input-low").value),
        creation_maximum: Number(creation_slider.querySelector(".range-input-high").value),
        album_minimum: Number(album_slider.querySelector(".range-input-low").value),
        album_maximum: Number(album_slider.querySelector(".range-input-high").value),
        member_counts: collect_checked_values(".member-checkbox"),
        location_slugs: collect_checked_values(".place-checkbox"),
    };
};

// card_plays_a_selected_location answers whether the artist played at even one
// of the ticked places.
const card_plays_a_selected_location = (card_element, selected_location_slugs) =>
    card_element.dataset.locations
        .split(" ")
        .some((card_location_slug) => selected_location_slugs.has(card_location_slug));

// card_matches_filters applies the four categories to a single card.
const card_matches_filters = (card_element, active_filters) => {
    const creation_year = Number(card_element.dataset.creationYear);
    if (creation_year < active_filters.creation_minimum ||
        creation_year > active_filters.creation_maximum) {
        return false;
    }

    // A first album year of 0 means the server could not read the date. Letting
    // it through is friendlier than making the artist vanish for a reason
    // nobody looking at the page could work out.
    const album_year = Number(card_element.dataset.albumYear);
    if (album_year !== 0 &&
        (album_year < active_filters.album_minimum ||
         album_year > active_filters.album_maximum)) {
        return false;
    }

    // An empty set means nothing was ticked in that category, and a category
    // with nothing ticked does not filter.
    if (active_filters.member_counts.size > 0 &&
        !active_filters.member_counts.has(card_element.dataset.memberCount)) {
        return false;
    }

    if (active_filters.location_slugs.size > 0 &&
        !card_plays_a_selected_location(card_element, active_filters.location_slugs)) {
        return false;
    }

    return true;
};

// apply_filters runs every card past the current sidebar state, then updates the
// count line and the empty state message to match.
const apply_filters = () => {
    // The search owns the grid while there is text in the search box, so a
    // stray event from the sidebar cannot reach the cards behind its back.
    if (!filters_drive_the_grid) {
        return;
    }

    const active_filters = read_active_filters();

    let matching_card_count = 0;
    artist_cards.forEach((card_element) => {
        const card_matches = card_matches_filters(card_element, active_filters);

        card_element.hidden = !card_matches;
        if (card_matches) {
            matching_card_count += 1;
        }
    });

    result_count_element.textContent =
        matching_card_count + " of " + artist_cards.length + " artists";

    // When nothing is left the grid is taken out of the flow altogether, so the
    // message stands in its place rather than underneath a blank gap.
    empty_state_element.hidden = matching_card_count > 0;
    artist_grid.hidden = matching_card_count === 0;
};


/* --------------------------------------------------------------------------
   Dual handle range sliders

   Each slider is two range inputs lying on top of one another over a single
   track. Keeping the low value at or below the high value is what makes the
   pair behave like one control with two handles.
   -------------------------------------------------------------------------- */

// refresh_range_slider redraws everything about a slider that depends on its
// current values: the "1970 - 2005" readout, the highlighted part of the track,
// and which of the two handles can be grabbed when they overlap.
const refresh_range_slider = (range_slider_element) => {
    const low_input = range_slider_element.querySelector(".range-input-low");
    const high_input = range_slider_element.querySelector(".range-input-high");
    const readout_low_element = range_slider_element.querySelector(".range-readout-low");
    const readout_high_element = range_slider_element.querySelector(".range-readout-high");
    const track_fill_element = range_slider_element.querySelector(".range-track-fill");

    const slider_minimum = Number(low_input.min);
    const slider_maximum = Number(low_input.max);
    const low_value = Number(low_input.value);
    const high_value = Number(high_input.value);

    readout_low_element.textContent = low_value;
    readout_high_element.textContent = high_value;

    // A slider whose two ends are the same year has no span to divide by, which
    // would give NaN, so in that case the whole track is filled instead.
    const slider_span = slider_maximum - slider_minimum;
    let low_percentage = 0;
    let high_percentage = 100;
    if (slider_span > 0) {
        low_percentage = ((low_value - slider_minimum) / slider_span) * 100;
        high_percentage = ((high_value - slider_minimum) / slider_span) * 100;
    }

    track_fill_element.style.left = low_percentage + "%";
    track_fill_element.style.width = (high_percentage - low_percentage) + "%";

    // Once the low handle has been dragged into the right hand half of the track
    // it can end up sitting underneath the high handle, where it could no longer
    // be grabbed. Lifting it above the high input there keeps both handles
    // reachable wherever they are.
    low_input.style.zIndex = low_percentage > 50 ? "5" : "3";
};

// setup_range_slider wires up one slider so that neither handle can be dragged
// past the other, and so that moving either one re-filters the grid.
const setup_range_slider = (range_slider_element) => {
    const low_input = range_slider_element.querySelector(".range-input-low");
    const high_input = range_slider_element.querySelector(".range-input-high");

    low_input.addEventListener("input", () => {
        // Pushed back rather than swapped, so the handle simply stops at its
        // neighbour instead of the two changing places.
        if (Number(low_input.value) > Number(high_input.value)) {
            low_input.value = high_input.value;
        }

        refresh_range_slider(range_slider_element);
        apply_filters();
    });

    high_input.addEventListener("input", () => {
        if (Number(high_input.value) < Number(low_input.value)) {
            high_input.value = low_input.value;
        }

        refresh_range_slider(range_slider_element);
        apply_filters();
    });

    refresh_range_slider(range_slider_element);
};


/* --------------------------------------------------------------------------
   Country and place check boxes
   -------------------------------------------------------------------------- */

// refresh_country_checkbox puts a country box into one of its three states:
// ticked when every place under it is ticked, cleared when none of them is, and
// the dashed in between state when only some of them are.
const refresh_country_checkbox = (country_group_element) => {
    const country_checkbox = country_group_element.querySelector(".country-checkbox");
    const place_checkboxes = country_group_element.querySelectorAll(".place-checkbox");

    let ticked_place_count = 0;
    place_checkboxes.forEach((place_checkbox) => {
        if (place_checkbox.checked) {
            ticked_place_count += 1;
        }
    });

    country_checkbox.checked = place_checkboxes.length > 0 &&
        ticked_place_count === place_checkboxes.length;
    country_checkbox.indeterminate = ticked_place_count > 0 &&
        ticked_place_count < place_checkboxes.length;
};

// setup_country_group wires up one country and the places inside it, in both
// directions: the country ticks its places, and the places decide how the
// country box looks.
const setup_country_group = (country_group_element) => {
    const country_checkbox = country_group_element.querySelector(".country-checkbox");
    const country_option_label = country_group_element.querySelector(".country-option");
    const place_checkboxes = country_group_element.querySelectorAll(".place-checkbox");

    // A click anywhere on the summary line folds the group open or shut. The
    // country's own check box sits on that line, so its clicks are stopped here
    // to keep ticking a country from also collapsing it.
    country_option_label.addEventListener("click", (click_event) => {
        click_event.stopPropagation();
    });

    country_checkbox.addEventListener("change", () => {
        place_checkboxes.forEach((place_checkbox) => {
            place_checkbox.checked = country_checkbox.checked;
        });

        // Ticking or clearing the country settles every place under it, so the
        // in between state no longer applies.
        country_checkbox.indeterminate = false;

        apply_filters();
    });

    place_checkboxes.forEach((place_checkbox) => {
        place_checkbox.addEventListener("change", () => {
            refresh_country_checkbox(country_group_element);
            apply_filters();
        });
    });
};


/* --------------------------------------------------------------------------
   Reset, the narrow screen toggle, and start up
   -------------------------------------------------------------------------- */

// set_filters_enabled hands the grid over to the search bar and takes it back.
// Every control is switched off while the search is in charge, so a sidebar that
// is not driving the grid also emits no change events at all, and the panel is
// dimmed so that it reads as being out of play rather than broken.
//
// The reset button is deliberately left alive: it clears the search box as well,
// so it stays the one way back to a clean page from any state.
const set_filters_enabled = (filters_are_enabled) => {
    filters_drive_the_grid = filters_are_enabled;

    filters_sidebar.classList.toggle("filters-disabled", !filters_are_enabled);

    document
        .querySelectorAll(".range-input, .member-checkbox, .place-checkbox, .country-checkbox")
        .forEach((filter_control) => {
            filter_control.disabled = !filters_are_enabled;
        });
};

// reset_all_filters puts every control back to the state the page was first
// rendered in: both sliders at their full width, every box cleared, and the
// search box empty.
const reset_all_filters = () => {
    document.querySelectorAll(".range-slider").forEach((range_slider_element) => {
        const low_input = range_slider_element.querySelector(".range-input-low");
        const high_input = range_slider_element.querySelector(".range-input-high");

        low_input.value = low_input.min;
        high_input.value = high_input.max;

        refresh_range_slider(range_slider_element);
    });

    document
        .querySelectorAll(".member-checkbox, .place-checkbox, .country-checkbox")
        .forEach((checkbox) => {
            checkbox.checked = false;
            checkbox.indeterminate = false;
        });

    // Emptying the box is announced rather than done quietly, so the search
    // script hears it through the same event a visitor deleting the text would
    // raise, and hands the grid back through its own normal path.
    if (search_input_element !== null && search_input_element.value !== "") {
        search_input_element.value = "";
        search_input_element.dispatchEvent(new Event("input"));
    }

    apply_filters();
};

// publish_header_height measures the sticky header bar and writes its height
// into the --header_height custom property, which is what the stylesheet rests
// the sidebar against so that the panel comes to a stop just under the bar
// instead of behind it.
//
// It is measured rather than written into the stylesheet as a number, because
// the heading inside the bar is sized in vw units and the search box stacks
// under it on a narrow screen, so the bar is not the same height on every
// screen. A ResizeObserver keeps the value right afterwards as well, since the
// bar changes height when the window is resized and when the web font arrives
// and reflows the heading.
const publish_header_height = () => {
    const measured_header_height = page_header_element.offsetHeight;

    document.documentElement.style.setProperty(
        "--header_height",
        measured_header_height + "px",
    );
};

const setup_header_height = () => {
    publish_header_height();

    new ResizeObserver(publish_header_height).observe(page_header_element);
};

// setup_filters_toggle wires the button that folds the sidebar in and out on
// screens too narrow to show it beside the grid.
const setup_filters_toggle = () => {
    filters_toggle_button.addEventListener("click", () => {
        const sidebar_is_open = filters_sidebar.classList.toggle("filters-open");

        filters_toggle_button.setAttribute("aria-expanded", String(sidebar_is_open));
    });
};

const initialise_filters = () => {
    // The script belongs to the home page. Bailing out rather than throwing
    // keeps it harmless if it is ever pulled into another page.
    if (artist_grid === null) {
        return;
    }

    artist_cards = Array.from(artist_grid.querySelectorAll(".card"));

    document.querySelectorAll(".range-slider").forEach(setup_range_slider);
    document.querySelectorAll(".country-group").forEach(setup_country_group);
    document.querySelectorAll(".member-checkbox").forEach((member_checkbox) => {
        member_checkbox.addEventListener("change", apply_filters);
    });

    filters_reset_button.addEventListener("click", reset_all_filters);
    setup_filters_toggle();
    setup_header_height();

    // One run at the start, so the count line is right before the visitor has
    // touched anything.
    apply_filters();
};

initialise_filters();

// The two things static/js/search.js needs from here, and the only two things
// this file offers anyone: switch the sidebar off while the search drives the
// grid, and re-run the filters once the search hands it back.
window.groupie_filters = {
    apply: apply_filters,
    set_enabled: set_filters_enabled,
};
