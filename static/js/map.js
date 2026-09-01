// Reads the concert stops the Go template embedded in the page and draws
// them on a Leaflet map: one numbered marker per stop in tour order, and a
// line tracing the route between them.

const map_container = document.getElementById("concert-map");
const map_data_element = document.getElementById("concert-map-data");

// A stop with no coordinates would be plotted at 0,0 in the Atlantic, so
// only the ones that were successfully geocoded are kept.
let concert_stops = [];
try {
    concert_stops = JSON.parse(map_data_element.textContent).filter(
        (concert_stop) => concert_stop.hasCoordinates
    );
} catch (parse_error) {
    concert_stops = [];
}

if (concert_stops.length === 0) {
    map_container.textContent = "Map unavailable for this artist.";
    map_container.classList.add("map-unavailable");
} else {
    const concert_map = L.map("concert-map");

    L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
        attribution: "&copy; OpenStreetMap contributors",
        maxZoom: 19,
    }).addTo(concert_map);

    const route_points = [];

    concert_stops.forEach((concert_stop) => {
        const stop_position = [concert_stop.latitude, concert_stop.longitude];
        route_points.push(stop_position);

        const numbered_icon = L.divIcon({
            className: "concert-marker",
            html: String(concert_stop.order),
            iconSize: [28, 28],
            iconAnchor: [14, 14],
        });

        L.marker(stop_position, { icon: numbered_icon })
            .addTo(concert_map)
            .bindPopup(
                "<strong>" + concert_stop.location + "</strong><br>" +
                concert_stop.dates.join("<br>")
            );
    });

    L.polyline(route_points, { color: "#1db954", weight: 2 }).addTo(concert_map);

    concert_map.fitBounds(route_points, { padding: [30, 30] });
}