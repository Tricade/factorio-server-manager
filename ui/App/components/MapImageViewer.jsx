import React, {useCallback, useEffect, useRef, useState} from "react";
import * as ReactDom from "react-dom";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faExpand, faMagnifyingGlassMinus, faMagnifyingGlassPlus, faRotateLeft, faXmark} from "@fortawesome/free-solid-svg-icons";
import {focusableElements, lockBodyScroll, trapFocus} from "./overlay";
import mapOverlayGeometryHelpers from "./mapOverlayGeometry.cjs";

const {mapOverlayGeometry} = mapOverlayGeometryHelpers;

const modalRoot = document.getElementById("modal-root");
const minimumZoom = 1;
const maximumZoom = 12;
export const entityDetailZoom = 1.5;

const entityCategories = [
    {id: "production", label: "Production", color: "#ffb34d", types: new Set(["assembling-machine", "furnace", "mining-drill", "lab", "rocket-silo", "agricultural-tower", "beacon"])},
    {id: "logistics", label: "Logistics", color: "#56c8ff", types: new Set(["transport-belt", "underground-belt", "splitter", "loader", "loader-1x1", "inserter"])},
    {id: "rail", label: "Rail", color: "#d7dce2", types: new Set(["straight-rail", "half-diagonal-rail", "curved-rail-a", "curved-rail-b", "rail-signal", "rail-chain-signal", "train-stop", "locomotive", "cargo-wagon", "fluid-wagon", "artillery-wagon"] )},
    {id: "power", label: "Power", color: "#ffe36e", types: new Set(["electric-pole", "generator", "solar-panel", "accumulator", "reactor", "boiler", "burner-generator", "fusion-generator"])},
    {id: "fluids", label: "Fluids", color: "#8d8cff", types: new Set(["pipe", "pipe-to-ground", "pump", "storage-tank", "offshore-pump"] )},
    {id: "defense", label: "Defense", color: "#ff657a", types: new Set(["ammo-turret", "electric-turret", "fluid-turret", "artillery-turret", "wall", "gate", "radar"] )},
    {id: "storage", label: "Storage", color: "#7ee29c", types: new Set(["container", "logistic-container", "linked-container"] )},
    {id: "other", label: "Other", color: "#c39cff", types: new Set()}
];

const entityCategory = type => entityCategories.find(category => category.types.has(type)) || entityCategories[entityCategories.length - 1];

const surfaceBounds = surface => {
    if (surface?.view_bounds_available !== true || !Number.isFinite(surface?.pixels_per_tile) || surface.pixels_per_tile <= 0) return null;
    const bounds = surface?.render_bounds || surface?.chart_bounds || surface?.bounds || {};
    const leftTop = bounds.left_top || bounds.leftTop || {};
    const rightBottom = bounds.right_bottom || bounds.rightBottom || {};
    const minX = bounds.min_x ?? bounds.minX ?? leftTop.x ?? surface?.render_min_x ?? surface?.chart_min_x ?? surface?.view_min_tile_x;
    const minY = bounds.min_y ?? bounds.minY ?? leftTop.y ?? surface?.render_min_y ?? surface?.chart_min_y ?? surface?.view_min_tile_y;
    const maxX = bounds.max_x ?? bounds.maxX ?? rightBottom.x ?? surface?.render_max_x ?? surface?.chart_max_x ?? surface?.view_max_tile_x;
    const maxY = bounds.max_y ?? bounds.maxY ?? rightBottom.y ?? surface?.render_max_y ?? surface?.chart_max_y ?? surface?.view_max_tile_y;
    return [minX, minY, maxX, maxY].every(Number.isFinite) && minX <= maxX && minY <= maxY
        ? {minX, minY, maxX, maxY}
        : null;
};

const MapEntityCanvas = ({entities, surface, view, detailZoom = entityDetailZoom, detailedSmallSurface = false}) => {
    const canvasRef = useRef(null);
    const bounds = surfaceBounds(surface);

    useEffect(() => {
        const canvas = canvasRef.current;
        if (!canvas || !bounds || !entities?.length || !surface?.width || !surface?.height) return;
        const geometry = mapOverlayGeometry(surface, detailedSmallSurface);
        if (!geometry) return;
        const {outputScale, width, height} = geometry;
        canvas.width = width;
        canvas.height = height;
        const context = canvas.getContext("2d", {alpha: true});
        context.clearRect(0, 0, width, height);
        context.globalAlpha = 0.78;
        context.lineWidth = Math.min(2, Math.max(0.45, outputScale * 0.08));
        context.strokeStyle = "rgba(5, 9, 13, 0.72)";
        const scaleX = surface.pixels_per_tile * outputScale;
        const scaleY = surface.pixels_per_tile * outputScale;
        const grouped = new Map(entityCategories.map(category => [category.id, []]));
        entities.forEach(entity => grouped.get(entityCategory(entity.type).id).push(entity));

        entityCategories.forEach(category => {
            const matching = grouped.get(category.id);
            context.fillStyle = category.color;
            for (let offset = 0; offset < matching.length; offset += 2000) {
                context.beginPath();
                matching.slice(offset, offset + 2000).forEach(entity => {
                    const box = entity.bounding_box;
                    const x = (box.left_top.x - bounds.minX) * scaleX;
                    const y = (box.left_top.y - bounds.minY) * scaleY;
                    const entityWidth = Math.max(0.7 * outputScale, (box.right_bottom.x - box.left_top.x) * scaleX);
                    const entityHeight = Math.max(0.7 * outputScale, (box.right_bottom.y - box.left_top.y) * scaleY);
                    context.rect(x, y, entityWidth, entityHeight);
                });
                context.fill();
                context.stroke();
            }
        });
    }, [bounds?.minX, bounds?.minY, bounds?.maxX, bounds?.maxY, detailedSmallSurface, entities, surface?.height, surface?.pixels_per_tile, surface?.width]);

    if (!bounds || !entities?.length) return null;
    return <canvas
        ref={canvasRef}
        className={`ui-map-entity-overlay${view.zoom >= detailZoom ? " is-visible" : ""}`}
        aria-hidden="true"
    />;
};

const MapEntityLegend = ({overlay, zoom, detailZoom = entityDetailZoom}) => {
    const counts = React.useMemo(() => {
        const result = new Map();
        (overlay?.entities || []).forEach(entity => {
            const category = entityCategory(entity.type);
            result.set(category.id, (result.get(category.id) || 0) + 1);
        });
        return result;
    }, [overlay?.entities]);

    if (!overlay?.surface?.entities_available || !surfaceBounds(overlay.surface)) return null;
    let status = null;
    if (zoom < detailZoom) status = `Zoom to ${Math.round(detailZoom * 100)}% for building detail`;
    else if (overlay.isLoading) status = "Loading building detail…";
    else if (overlay.error) status = "Building detail unavailable";
    else if (overlay.entities === null) status = "Loading building detail…";
    else if (!overlay.entities.length) status = "No building footprints in this snapshot";

    return <div className="ui-map-entity-legend" role="status">
        {status
            ? <span>{status}</span>
            : <>
                <strong>{overlay.entities.length.toLocaleString()} buildings</strong>
                {entityCategories.filter(category => counts.has(category.id)).map(category => <span key={category.id}>
                    <i style={{backgroundColor: category.color}} aria-hidden="true"/>{category.label}
                </span>)}
            </>}
        {overlay.surface.entity_truncated && !overlay.isLoading && <span title={`${overlay.surface.entity_total_count?.toLocaleString() || "More"} buildings exist in the source snapshot.`}>Partial detail</span>}
    </div>;
};

const clampZoom = value => Math.min(maximumZoom, Math.max(minimumZoom, value));

const MapImageCanvas = ({src, alt, view, setView, onFullscreen = null, isFullscreen = false, isPixelated = false, entityOverlay = null, detailZoom = entityDetailZoom}) => {
    const viewportRef = useRef(null);
    const dragRef = useRef(null);
    const [isDragging, setIsDragging] = useState(false);
    const [imageFailed, setImageFailed] = useState(false);
    const [stageSize, setStageSize] = useState(null);
    const surfaceWidth = Number(entityOverlay?.surface?.width);
    const surfaceHeight = Number(entityOverlay?.surface?.height);

    useEffect(() => setImageFailed(false), [src]);

    useEffect(() => {
        const viewport = viewportRef.current;
        if (!viewport || !(surfaceWidth > 0) || !(surfaceHeight > 0)) {
            setStageSize(null);
            return undefined;
        }
        const update = () => {
            const viewportWidth = viewport.clientWidth;
            const viewportHeight = viewport.clientHeight;
            if (!viewportWidth || !viewportHeight) return;
            const imageRatio = surfaceWidth / surfaceHeight;
            const viewportRatio = viewportWidth / viewportHeight;
            const width = viewportRatio > imageRatio ? viewportHeight * imageRatio : viewportWidth;
            const height = viewportRatio > imageRatio ? viewportHeight : viewportWidth / imageRatio;
            setStageSize(current => current && Math.abs(current.width - width) < 0.5 && Math.abs(current.height - height) < 0.5
                ? current
                : {width, height});
        };
        update();
        if (typeof ResizeObserver !== "undefined") {
            const observer = new ResizeObserver(update);
            observer.observe(viewport);
            return () => observer.disconnect();
        }
        window.addEventListener("resize", update);
        return () => window.removeEventListener("resize", update);
    }, [surfaceHeight, surfaceWidth]);

    const zoomAt = useCallback((factor, clientX = null, clientY = null) => {
        const viewport = viewportRef.current;
        if (!viewport) return;
        const rect = viewport.getBoundingClientRect();
        const pointX = clientX === null ? 0 : clientX - rect.left - rect.width / 2;
        const pointY = clientY === null ? 0 : clientY - rect.top - rect.height / 2;
        setView(current => {
            const zoom = clampZoom(current.zoom * factor);
            if (zoom === minimumZoom) return {zoom, x: 0, y: 0};
            const ratio = zoom / current.zoom;
            return {
                zoom,
                x: pointX - (pointX - current.x) * ratio,
                y: pointY - (pointY - current.y) * ratio
            };
        });
    }, [setView]);

    useEffect(() => {
        const viewport = viewportRef.current;
        if (!viewport) return undefined;
        const handleWheel = event => {
            event.preventDefault();
            zoomAt(event.deltaY < 0 ? 1.2 : 1 / 1.2, event.clientX, event.clientY);
        };
        viewport.addEventListener("wheel", handleWheel, {passive: false});
        return () => viewport.removeEventListener("wheel", handleWheel);
    }, [zoomAt]);

    const startDrag = event => {
        if (event.button !== 0) return;
        event.currentTarget.setPointerCapture(event.pointerId);
        dragRef.current = {pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY, x: view.x, y: view.y};
        setIsDragging(true);
    };

    const moveDrag = event => {
        const drag = dragRef.current;
        if (!drag || drag.pointerId !== event.pointerId) return;
        setView(current => ({
            ...current,
            x: drag.x + event.clientX - drag.clientX,
            y: drag.y + event.clientY - drag.clientY
        }));
    };

    const endDrag = event => {
        if (dragRef.current?.pointerId !== event.pointerId) return;
        dragRef.current = null;
        setIsDragging(false);
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    };

    const reset = () => setView({zoom: minimumZoom, x: 0, y: 0});

    const handleKeyDown = event => {
        if (["+", "="].includes(event.key)) {
            event.preventDefault();
            zoomAt(1.25);
        } else if (["-", "_"].includes(event.key)) {
            event.preventDefault();
            zoomAt(1 / 1.25);
        } else if (["0", "Home"].includes(event.key)) {
            event.preventDefault();
            reset();
        } else if (["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)) {
            event.preventDefault();
            const distance = event.shiftKey ? 80 : 32;
            setView(current => ({
                ...current,
                x: current.x + (event.key === "ArrowLeft" ? distance : event.key === "ArrowRight" ? -distance : 0),
                y: current.y + (event.key === "ArrowUp" ? distance : event.key === "ArrowDown" ? -distance : 0)
            }));
        }
    };

    return <div className={`ui-map-viewer${isFullscreen ? " ui-map-viewer--fullscreen" : ""}`}>
        <div className="ui-map-viewer__controls" role="group" aria-label="Map zoom controls">
            <button type="button" onClick={() => zoomAt(1 / 1.25)} disabled={view.zoom <= minimumZoom} title="Zoom out" aria-label="Zoom out">
                <FontAwesomeIcon icon={faMagnifyingGlassMinus}/>
            </button>
            <output aria-label="Map zoom level">{Math.round(view.zoom * 100)}%</output>
            <button type="button" onClick={() => zoomAt(1.25)} disabled={view.zoom >= maximumZoom} title="Zoom in" aria-label="Zoom in">
                <FontAwesomeIcon icon={faMagnifyingGlassPlus}/>
            </button>
            <button type="button" onClick={reset} disabled={view.zoom === minimumZoom && view.x === 0 && view.y === 0} title="Fit map" aria-label="Fit map">
                <FontAwesomeIcon icon={faRotateLeft}/>
            </button>
            {onFullscreen && <button type="button" onClick={onFullscreen} title="Open fullscreen map" aria-label="Open fullscreen map">
                <FontAwesomeIcon icon={faExpand}/>
            </button>}
        </div>
        <div
            ref={viewportRef}
            className={`ui-map-viewer__canvas${isDragging ? " is-dragging" : ""}`}
            onPointerDown={startDrag}
            onPointerMove={moveDrag}
            onPointerUp={endDrag}
            onPointerCancel={endDrag}
            onDoubleClick={event => zoomAt(view.zoom >= 2 ? 1 / view.zoom : 2, event.clientX, event.clientY)}
            onKeyDown={handleKeyDown}
            role="region"
            tabIndex={0}
            aria-keyshortcuts="+ - 0 Home ArrowLeft ArrowRight ArrowUp ArrowDown"
            aria-label={`${alt}. Use the mouse wheel, plus and minus keys, or zoom buttons to zoom. Drag or use the arrow keys to move. Press 0 to fit the map.`}
        >
            {imageFailed && <div className="ui-map-viewer__error" role="alert">Map image could not be loaded.</div>}
            <div className="ui-map-viewer__stage-positioner">
                {stageSize && <div
                    className="ui-map-viewer__stage"
                    style={{
                        width: `${stageSize.width}px`,
                        height: `${stageSize.height}px`,
                        transform: `translate3d(${view.x}px, ${view.y}px, 0) scale(${view.zoom})`
                    }}
                >
                    <img
                        className={isPixelated ? "is-pixelated" : ""}
                        src={src}
                        alt={alt}
                        draggable="false"
                        onError={() => setImageFailed(true)}
                    />
                    <MapEntityCanvas entities={entityOverlay?.entities} surface={entityOverlay?.surface} view={view} detailZoom={detailZoom} detailedSmallSurface={isPixelated}/>
                </div>}
            </div>
        </div>
        <MapEntityLegend overlay={entityOverlay} zoom={view.zoom} detailZoom={detailZoom}/>
    </div>;
};

export const MapImageLightbox = ({src, alt, title, isOpen, close, view, setView, isPixelated = false, entityOverlay = null, detailZoom = entityDetailZoom}) => {
    const dialogRef = useRef(null);
    const closeRef = useRef(close);
    closeRef.current = close;

    useEffect(() => {
        if (!isOpen) return undefined;
        const previouslyFocused = document.activeElement;
        const unlockBodyScroll = lockBodyScroll();
        const focusDialog = window.requestAnimationFrame(() => {
            (focusableElements(dialogRef.current)[0] || dialogRef.current)?.focus();
        });
        const handleKeyDown = event => {
            if (event.key === "Escape") {
                event.preventDefault();
                closeRef.current();
                return;
            }
            trapFocus(event, dialogRef.current);
        };
        document.addEventListener("keydown", handleKeyDown);
        return () => {
            window.cancelAnimationFrame(focusDialog);
            document.removeEventListener("keydown", handleKeyDown);
            unlockBodyScroll();
            if (previouslyFocused instanceof HTMLElement && previouslyFocused.isConnected) previouslyFocused.focus();
        };
    }, [isOpen]);

    if (!isOpen) return null;
    return ReactDom.createPortal(
        <div ref={dialogRef} className="ui-map-lightbox" role="dialog" aria-modal="true" aria-label={`${title} fullscreen map`} tabIndex={-1} onPointerDown={event => {
            if (event.target === event.currentTarget) close();
        }}>
            <section className="ui-map-lightbox__panel">
                <header>
                    <div><span>Factory map</span><h2>{title}</h2></div>
                    <button type="button" onClick={close} title="Close fullscreen map" aria-label="Close fullscreen map">
                        <FontAwesomeIcon icon={faXmark}/>
                    </button>
                </header>
                <MapImageCanvas src={src} alt={alt} view={view} setView={setView} isFullscreen isPixelated={isPixelated} entityOverlay={entityOverlay} detailZoom={detailZoom}/>
            </section>
        </div>,
        modalRoot
    );
};

const MapImageViewer = props => <MapImageCanvas {...props}/>;

export default MapImageViewer;
