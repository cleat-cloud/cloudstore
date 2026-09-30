(() => {
  var __defProp = Object.defineProperty;
  var __export = (target, all) => {
    for (var name in all)
      __defProp(target, name, { get: all[name], enumerable: true });
  };

  // pkg/amarra/js/vendor/idiomorph.js
  var Idiomorph = (function() {
    "use strict";
    const noOp = () => {
    };
    const defaults = {
      morphStyle: "outerHTML",
      callbacks: {
        beforeNodeAdded: noOp,
        afterNodeAdded: noOp,
        beforeNodeMorphed: noOp,
        afterNodeMorphed: noOp,
        beforeNodeRemoved: noOp,
        afterNodeRemoved: noOp,
        beforeAttributeUpdated: noOp
      },
      head: {
        style: "merge",
        shouldPreserve: (elt) => elt.getAttribute("im-preserve") === "true",
        shouldReAppend: (elt) => elt.getAttribute("im-re-append") === "true",
        shouldRemove: noOp,
        afterHeadMorphed: noOp
      },
      restoreFocus: true
    };
    function morph2(oldNode, newContent, config = {}) {
      oldNode = normalizeElement(oldNode);
      const newNode = normalizeParent(newContent);
      const ctx = createMorphContext(oldNode, newNode, config);
      const morphedNodes = saveAndRestoreFocus(ctx, () => {
        return withHeadBlocking(
          ctx,
          oldNode,
          newNode,
          /** @param {MorphContext} ctx */
          (ctx2) => {
            if (ctx2.morphStyle === "innerHTML") {
              morphChildren(ctx2, oldNode, newNode);
              return Array.from(oldNode.childNodes);
            } else {
              return morphOuterHTML(ctx2, oldNode, newNode);
            }
          }
        );
      });
      ctx.pantry.remove();
      return morphedNodes;
    }
    function morphOuterHTML(ctx, oldNode, newNode) {
      const oldParent = normalizeParent(oldNode);
      morphChildren(
        ctx,
        oldParent,
        newNode,
        // these two optional params are the secret sauce
        oldNode,
        // start point for iteration
        oldNode.nextSibling
        // end point for iteration
      );
      return Array.from(oldParent.childNodes);
    }
    function saveAndRestoreFocus(ctx, fn) {
      if (!ctx.config.restoreFocus) return fn();
      let activeElement = (
        /** @type {HTMLInputElement|HTMLTextAreaElement|null} */
        document.activeElement
      );
      if (!(activeElement instanceof HTMLInputElement || activeElement instanceof HTMLTextAreaElement)) {
        return fn();
      }
      const { id: activeElementId, selectionStart, selectionEnd } = activeElement;
      const results = fn();
      if (activeElementId && activeElementId !== document.activeElement?.getAttribute("id")) {
        activeElement = ctx.target.querySelector(`[id="${activeElementId}"]`);
        activeElement?.focus();
      }
      if (activeElement && !activeElement.selectionEnd && selectionEnd) {
        activeElement.setSelectionRange(selectionStart, selectionEnd);
      }
      return results;
    }
    const morphChildren = /* @__PURE__ */ (function() {
      function morphChildren2(ctx, oldParent, newParent, insertionPoint = null, endPoint = null) {
        if (oldParent instanceof HTMLTemplateElement && newParent instanceof HTMLTemplateElement) {
          oldParent = oldParent.content;
          newParent = newParent.content;
        }
        insertionPoint ||= oldParent.firstChild;
        for (const newChild of newParent.childNodes) {
          if (insertionPoint && insertionPoint != endPoint) {
            const bestMatch = findBestMatch(
              ctx,
              newChild,
              insertionPoint,
              endPoint
            );
            if (bestMatch) {
              if (bestMatch !== insertionPoint) {
                removeNodesBetween(ctx, insertionPoint, bestMatch);
              }
              morphNode(bestMatch, newChild, ctx);
              insertionPoint = bestMatch.nextSibling;
              continue;
            }
          }
          if (newChild instanceof Element) {
            const newChildId = (
              /** @type {String} */
              newChild.getAttribute("id")
            );
            if (ctx.persistentIds.has(newChildId)) {
              const movedChild = moveBeforeById(
                oldParent,
                newChildId,
                insertionPoint,
                ctx
              );
              morphNode(movedChild, newChild, ctx);
              insertionPoint = movedChild.nextSibling;
              continue;
            }
          }
          const insertedNode = createNode(
            oldParent,
            newChild,
            insertionPoint,
            ctx
          );
          if (insertedNode) {
            insertionPoint = insertedNode.nextSibling;
          }
        }
        while (insertionPoint && insertionPoint != endPoint) {
          const tempNode = insertionPoint;
          insertionPoint = insertionPoint.nextSibling;
          removeNode(ctx, tempNode);
        }
      }
      function createNode(oldParent, newChild, insertionPoint, ctx) {
        if (ctx.callbacks.beforeNodeAdded(newChild) === false) return null;
        if (ctx.idMap.has(newChild)) {
          const newEmptyChild = document.createElement(
            /** @type {Element} */
            newChild.tagName
          );
          oldParent.insertBefore(newEmptyChild, insertionPoint);
          morphNode(newEmptyChild, newChild, ctx);
          ctx.callbacks.afterNodeAdded(newEmptyChild);
          return newEmptyChild;
        } else {
          const newClonedChild = document.importNode(newChild, true);
          oldParent.insertBefore(newClonedChild, insertionPoint);
          ctx.callbacks.afterNodeAdded(newClonedChild);
          return newClonedChild;
        }
      }
      const findBestMatch = /* @__PURE__ */ (function() {
        function findBestMatch2(ctx, node, startPoint, endPoint) {
          let softMatch = null;
          let nextSibling = node.nextSibling;
          let siblingSoftMatchCount = 0;
          let cursor = startPoint;
          while (cursor && cursor != endPoint) {
            if (isSoftMatch(cursor, node)) {
              if (isIdSetMatch(ctx, cursor, node)) {
                return cursor;
              }
              if (softMatch === null) {
                if (!ctx.idMap.has(cursor)) {
                  softMatch = cursor;
                }
              }
            }
            if (softMatch === null && nextSibling && isSoftMatch(cursor, nextSibling)) {
              siblingSoftMatchCount++;
              nextSibling = nextSibling.nextSibling;
              if (siblingSoftMatchCount >= 2) {
                softMatch = void 0;
              }
            }
            if (ctx.activeElementAndParents.includes(cursor)) break;
            cursor = cursor.nextSibling;
          }
          return softMatch || null;
        }
        function isIdSetMatch(ctx, oldNode, newNode) {
          let oldSet = ctx.idMap.get(oldNode);
          let newSet = ctx.idMap.get(newNode);
          if (!newSet || !oldSet) return false;
          for (const id of oldSet) {
            if (newSet.has(id)) {
              return true;
            }
          }
          return false;
        }
        function isSoftMatch(oldNode, newNode) {
          const oldElt = (
            /** @type {Element} */
            oldNode
          );
          const newElt = (
            /** @type {Element} */
            newNode
          );
          return oldElt.nodeType === newElt.nodeType && oldElt.tagName === newElt.tagName && // If oldElt has an `id` with possible state and it doesn't match newElt.id then avoid morphing.
          // We'll still match an anonymous node with an IDed newElt, though, because if it got this far,
          // its not persistent, and new nodes can't have any hidden state.
          // We can't use .id because of form input shadowing, and we can't count on .getAttribute's presence because it could be a document-fragment
          (!oldElt.getAttribute?.("id") || oldElt.getAttribute?.("id") === newElt.getAttribute?.("id"));
        }
        return findBestMatch2;
      })();
      function removeNode(ctx, node) {
        if (ctx.idMap.has(node)) {
          moveBefore(ctx.pantry, node, null);
        } else {
          if (ctx.callbacks.beforeNodeRemoved(node) === false) return;
          node.parentNode?.removeChild(node);
          ctx.callbacks.afterNodeRemoved(node);
        }
      }
      function removeNodesBetween(ctx, startInclusive, endExclusive) {
        let cursor = startInclusive;
        while (cursor && cursor !== endExclusive) {
          let tempNode = (
            /** @type {Node} */
            cursor
          );
          cursor = cursor.nextSibling;
          removeNode(ctx, tempNode);
        }
        return cursor;
      }
      function moveBeforeById(parentNode, id, after, ctx) {
        const target = (
          /** @type {Element} - will always be found */
          // ctx.target.id unsafe because of form input shadowing
          // ctx.target could be a document fragment which doesn't have `getAttribute`
          ctx.target.getAttribute?.("id") === id && ctx.target || ctx.target.querySelector(`[id="${id}"]`) || ctx.pantry.querySelector(`[id="${id}"]`)
        );
        removeElementFromAncestorsIdMaps(target, ctx);
        moveBefore(parentNode, target, after);
        return target;
      }
      function removeElementFromAncestorsIdMaps(element, ctx) {
        const id = (
          /** @type {String} */
          element.getAttribute("id")
        );
        while (element = element.parentNode) {
          let idSet = ctx.idMap.get(element);
          if (idSet) {
            idSet.delete(id);
            if (!idSet.size) {
              ctx.idMap.delete(element);
            }
          }
        }
      }
      function moveBefore(parentNode, element, after) {
        if (parentNode.moveBefore) {
          try {
            parentNode.moveBefore(element, after);
          } catch (e) {
            parentNode.insertBefore(element, after);
          }
        } else {
          parentNode.insertBefore(element, after);
        }
      }
      return morphChildren2;
    })();
    const morphNode = /* @__PURE__ */ (function() {
      function morphNode2(oldNode, newContent, ctx) {
        if (ctx.ignoreActive && oldNode === document.activeElement) {
          return null;
        }
        if (ctx.callbacks.beforeNodeMorphed(oldNode, newContent) === false) {
          return oldNode;
        }
        if (oldNode instanceof HTMLHeadElement && ctx.head.ignore) {
        } else if (oldNode instanceof HTMLHeadElement && ctx.head.style !== "morph") {
          handleHeadElement(
            oldNode,
            /** @type {HTMLHeadElement} */
            newContent,
            ctx
          );
        } else {
          morphAttributes(oldNode, newContent, ctx);
          if (!ignoreValueOfActiveElement(oldNode, ctx)) {
            morphChildren(ctx, oldNode, newContent);
          }
        }
        ctx.callbacks.afterNodeMorphed(oldNode, newContent);
        return oldNode;
      }
      function morphAttributes(oldNode, newNode, ctx) {
        let type = newNode.nodeType;
        if (type === 1) {
          const oldElt = (
            /** @type {Element} */
            oldNode
          );
          const newElt = (
            /** @type {Element} */
            newNode
          );
          const oldAttributes = oldElt.attributes;
          const newAttributes = newElt.attributes;
          for (const newAttribute of newAttributes) {
            if (ignoreAttribute(newAttribute.name, oldElt, "update", ctx)) {
              continue;
            }
            if (oldElt.getAttribute(newAttribute.name) !== newAttribute.value) {
              oldElt.setAttribute(newAttribute.name, newAttribute.value);
            }
          }
          for (let i = oldAttributes.length - 1; 0 <= i; i--) {
            const oldAttribute = oldAttributes[i];
            if (!oldAttribute) continue;
            if (!newElt.hasAttribute(oldAttribute.name)) {
              if (ignoreAttribute(oldAttribute.name, oldElt, "remove", ctx)) {
                continue;
              }
              oldElt.removeAttribute(oldAttribute.name);
            }
          }
          if (!ignoreValueOfActiveElement(oldElt, ctx)) {
            syncInputValue(oldElt, newElt, ctx);
          }
        }
        if (type === 8 || type === 3) {
          if (oldNode.nodeValue !== newNode.nodeValue) {
            oldNode.nodeValue = newNode.nodeValue;
          }
        }
      }
      function syncInputValue(oldElement, newElement, ctx) {
        if (oldElement instanceof HTMLInputElement && newElement instanceof HTMLInputElement && newElement.type !== "file") {
          let newValue = newElement.value;
          let oldValue = oldElement.value;
          syncBooleanAttribute(oldElement, newElement, "checked", ctx);
          syncBooleanAttribute(oldElement, newElement, "disabled", ctx);
          if (!newElement.hasAttribute("value")) {
            if (!ignoreAttribute("value", oldElement, "remove", ctx)) {
              oldElement.value = "";
              oldElement.removeAttribute("value");
            }
          } else if (oldValue !== newValue) {
            if (!ignoreAttribute("value", oldElement, "update", ctx)) {
              oldElement.setAttribute("value", newValue);
              oldElement.value = newValue;
            }
          }
        } else if (oldElement instanceof HTMLOptionElement && newElement instanceof HTMLOptionElement) {
          syncBooleanAttribute(oldElement, newElement, "selected", ctx);
        } else if (oldElement instanceof HTMLTextAreaElement && newElement instanceof HTMLTextAreaElement) {
          let newValue = newElement.value;
          let oldValue = oldElement.value;
          if (ignoreAttribute("value", oldElement, "update", ctx)) {
            return;
          }
          if (newValue !== oldValue) {
            oldElement.value = newValue;
          }
          if (oldElement.firstChild && oldElement.firstChild.nodeValue !== newValue) {
            oldElement.firstChild.nodeValue = newValue;
          }
        }
      }
      function syncBooleanAttribute(oldElement, newElement, attributeName, ctx) {
        const newLiveValue = newElement[attributeName], oldLiveValue = oldElement[attributeName];
        if (newLiveValue !== oldLiveValue) {
          const ignoreUpdate = ignoreAttribute(
            attributeName,
            oldElement,
            "update",
            ctx
          );
          if (!ignoreUpdate) {
            oldElement[attributeName] = newElement[attributeName];
          }
          if (newLiveValue) {
            if (!ignoreUpdate) {
              oldElement.setAttribute(attributeName, "");
            }
          } else {
            if (!ignoreAttribute(attributeName, oldElement, "remove", ctx)) {
              oldElement.removeAttribute(attributeName);
            }
          }
        }
      }
      function ignoreAttribute(attr, element, updateType, ctx) {
        if (attr === "value" && ctx.ignoreActiveValue && element === document.activeElement) {
          return true;
        }
        return ctx.callbacks.beforeAttributeUpdated(attr, element, updateType) === false;
      }
      function ignoreValueOfActiveElement(possibleActiveElement, ctx) {
        return !!ctx.ignoreActiveValue && possibleActiveElement === document.activeElement && possibleActiveElement !== document.body;
      }
      return morphNode2;
    })();
    function withHeadBlocking(ctx, oldNode, newNode, callback) {
      if (ctx.head.block) {
        const oldHead = oldNode.querySelector("head");
        const newHead = newNode.querySelector("head");
        if (oldHead && newHead) {
          const promises = handleHeadElement(oldHead, newHead, ctx);
          return Promise.all(promises).then(() => {
            const newCtx = Object.assign(ctx, {
              head: {
                block: false,
                ignore: true
              }
            });
            return callback(newCtx);
          });
        }
      }
      return callback(ctx);
    }
    function handleHeadElement(oldHead, newHead, ctx) {
      let added = [];
      let removed = [];
      let preserved = [];
      let nodesToAppend = [];
      let srcToNewHeadNodes = /* @__PURE__ */ new Map();
      for (const newHeadChild of newHead.children) {
        srcToNewHeadNodes.set(newHeadChild.outerHTML, newHeadChild);
      }
      for (const currentHeadElt of oldHead.children) {
        let inNewContent = srcToNewHeadNodes.has(currentHeadElt.outerHTML);
        let isReAppended = ctx.head.shouldReAppend(currentHeadElt);
        let isPreserved = ctx.head.shouldPreserve(currentHeadElt);
        if (inNewContent || isPreserved) {
          if (isReAppended) {
            removed.push(currentHeadElt);
          } else {
            srcToNewHeadNodes.delete(currentHeadElt.outerHTML);
            preserved.push(currentHeadElt);
          }
        } else {
          if (ctx.head.style === "append") {
            if (isReAppended) {
              removed.push(currentHeadElt);
              nodesToAppend.push(currentHeadElt);
            }
          } else {
            if (ctx.head.shouldRemove(currentHeadElt) !== false) {
              removed.push(currentHeadElt);
            }
          }
        }
      }
      nodesToAppend.push(...srcToNewHeadNodes.values());
      let promises = [];
      for (const newNode of nodesToAppend) {
        let newElt = (
          /** @type {ChildNode} */
          document.createRange().createContextualFragment(newNode.outerHTML).firstChild
        );
        if (ctx.callbacks.beforeNodeAdded(newElt) !== false) {
          if ("href" in newElt && newElt.href || "src" in newElt && newElt.src) {
            let resolve;
            let promise = new Promise(function(_resolve) {
              resolve = _resolve;
            });
            newElt.addEventListener("load", function() {
              resolve();
            });
            promises.push(promise);
          }
          oldHead.appendChild(newElt);
          ctx.callbacks.afterNodeAdded(newElt);
          added.push(newElt);
        }
      }
      for (const removedElement of removed) {
        if (ctx.callbacks.beforeNodeRemoved(removedElement) !== false) {
          oldHead.removeChild(removedElement);
          ctx.callbacks.afterNodeRemoved(removedElement);
        }
      }
      ctx.head.afterHeadMorphed(oldHead, {
        added,
        kept: preserved,
        removed
      });
      return promises;
    }
    const createMorphContext = /* @__PURE__ */ (function() {
      function createMorphContext2(oldNode, newContent, config) {
        const { persistentIds, idMap } = createIdMaps(oldNode, newContent);
        const mergedConfig = mergeDefaults(config);
        const morphStyle = mergedConfig.morphStyle || "outerHTML";
        if (!["innerHTML", "outerHTML"].includes(morphStyle)) {
          throw `Do not understand how to morph style ${morphStyle}`;
        }
        return {
          target: oldNode,
          newContent,
          config: mergedConfig,
          morphStyle,
          ignoreActive: mergedConfig.ignoreActive,
          ignoreActiveValue: mergedConfig.ignoreActiveValue,
          restoreFocus: mergedConfig.restoreFocus,
          idMap,
          persistentIds,
          pantry: createPantry(),
          activeElementAndParents: createActiveElementAndParents(oldNode),
          callbacks: mergedConfig.callbacks,
          head: mergedConfig.head
        };
      }
      function mergeDefaults(config) {
        let finalConfig = Object.assign({}, defaults);
        Object.assign(finalConfig, config);
        finalConfig.callbacks = Object.assign(
          {},
          defaults.callbacks,
          config.callbacks
        );
        finalConfig.head = Object.assign({}, defaults.head, config.head);
        return finalConfig;
      }
      function createPantry() {
        const pantry = document.createElement("div");
        pantry.hidden = true;
        document.body.insertAdjacentElement("afterend", pantry);
        return pantry;
      }
      function createActiveElementAndParents(oldNode) {
        let activeElementAndParents = [];
        let elt = document.activeElement;
        if (elt?.tagName !== "BODY" && oldNode.contains(elt)) {
          while (elt) {
            activeElementAndParents.push(elt);
            if (elt === oldNode) break;
            elt = elt.parentElement;
          }
        }
        return activeElementAndParents;
      }
      function findIdElements(root) {
        let elements = Array.from(root.querySelectorAll("[id]"));
        if (root.getAttribute?.("id")) {
          elements.push(root);
        }
        return elements;
      }
      function populateIdMapWithTree(idMap, persistentIds, root, elements) {
        for (const elt of elements) {
          const id = (
            /** @type {String} */
            elt.getAttribute("id")
          );
          if (persistentIds.has(id)) {
            let current = elt;
            while (current) {
              let idSet = idMap.get(current);
              if (idSet == null) {
                idSet = /* @__PURE__ */ new Set();
                idMap.set(current, idSet);
              }
              idSet.add(id);
              if (current === root) break;
              current = current.parentElement;
            }
          }
        }
      }
      function createIdMaps(oldContent, newContent) {
        const oldIdElements = findIdElements(oldContent);
        const newIdElements = findIdElements(newContent);
        const persistentIds = createPersistentIds(oldIdElements, newIdElements);
        let idMap = /* @__PURE__ */ new Map();
        populateIdMapWithTree(idMap, persistentIds, oldContent, oldIdElements);
        const newRoot = newContent.__idiomorphRoot || newContent;
        populateIdMapWithTree(idMap, persistentIds, newRoot, newIdElements);
        return { persistentIds, idMap };
      }
      function createPersistentIds(oldIdElements, newIdElements) {
        let duplicateIds = /* @__PURE__ */ new Set();
        let oldIdTagNameMap = /* @__PURE__ */ new Map();
        for (const { id, tagName } of oldIdElements) {
          if (oldIdTagNameMap.has(id)) {
            duplicateIds.add(id);
          } else {
            oldIdTagNameMap.set(id, tagName);
          }
        }
        let persistentIds = /* @__PURE__ */ new Set();
        for (const { id, tagName } of newIdElements) {
          if (persistentIds.has(id)) {
            duplicateIds.add(id);
          } else if (oldIdTagNameMap.get(id) === tagName) {
            persistentIds.add(id);
          }
        }
        for (const id of duplicateIds) {
          persistentIds.delete(id);
        }
        return persistentIds;
      }
      return createMorphContext2;
    })();
    const { normalizeElement, normalizeParent } = /* @__PURE__ */ (function() {
      const generatedByIdiomorph = /* @__PURE__ */ new WeakSet();
      function normalizeElement2(content) {
        if (content instanceof Document) {
          return content.documentElement;
        } else {
          return content;
        }
      }
      function normalizeParent2(newContent) {
        if (newContent == null) {
          return document.createElement("div");
        } else if (typeof newContent === "string") {
          return normalizeParent2(parseContent(newContent));
        } else if (generatedByIdiomorph.has(
          /** @type {Element} */
          newContent
        )) {
          return (
            /** @type {Element} */
            newContent
          );
        } else if (newContent instanceof Node) {
          if (newContent.parentNode) {
            return (
              /** @type {any} */
              new SlicedParentNode(newContent)
            );
          } else {
            const dummyParent = document.createElement("div");
            dummyParent.append(newContent);
            return dummyParent;
          }
        } else {
          const dummyParent = document.createElement("div");
          for (const elt of [...newContent]) {
            dummyParent.append(elt);
          }
          return dummyParent;
        }
      }
      class SlicedParentNode {
        /** @param {Node} node */
        constructor(node) {
          this.originalNode = node;
          this.realParentNode = /** @type {Element} */
          node.parentNode;
          this.previousSibling = node.previousSibling;
          this.nextSibling = node.nextSibling;
        }
        /** @returns {Node[]} */
        get childNodes() {
          const nodes = [];
          let cursor = this.previousSibling ? this.previousSibling.nextSibling : this.realParentNode.firstChild;
          while (cursor && cursor != this.nextSibling) {
            nodes.push(cursor);
            cursor = cursor.nextSibling;
          }
          return nodes;
        }
        /**
         * @param {string} selector
         * @returns {Element[]}
         */
        querySelectorAll(selector) {
          return this.childNodes.reduce(
            (results, node) => {
              if (node instanceof Element) {
                if (node.matches(selector)) results.push(node);
                const nodeList = node.querySelectorAll(selector);
                for (let i = 0; i < nodeList.length; i++) {
                  results.push(nodeList[i]);
                }
              }
              return results;
            },
            /** @type {Element[]} */
            []
          );
        }
        /**
         * @param {Node} node
         * @param {Node} referenceNode
         * @returns {Node}
         */
        insertBefore(node, referenceNode) {
          return this.realParentNode.insertBefore(node, referenceNode);
        }
        /**
         * @param {Node} node
         * @param {Node} referenceNode
         * @returns {Node}
         */
        moveBefore(node, referenceNode) {
          return this.realParentNode.moveBefore(node, referenceNode);
        }
        /**
         * for later use with populateIdMapWithTree to halt upwards iteration
         * @returns {Node}
         */
        get __idiomorphRoot() {
          return this.originalNode;
        }
      }
      function parseContent(newContent) {
        let parser = new DOMParser();
        let contentWithSvgsRemoved = newContent.replace(
          /<svg(\s[^>]*>|>)([\s\S]*?)<\/svg>/gim,
          ""
        );
        if (contentWithSvgsRemoved.match(/<\/html>/) || contentWithSvgsRemoved.match(/<\/head>/) || contentWithSvgsRemoved.match(/<\/body>/)) {
          let content = parser.parseFromString(newContent, "text/html");
          if (contentWithSvgsRemoved.match(/<\/html>/)) {
            generatedByIdiomorph.add(content);
            return content;
          } else {
            let htmlElement = content.firstChild;
            if (htmlElement) {
              generatedByIdiomorph.add(htmlElement);
            }
            return htmlElement;
          }
        } else {
          let responseDoc = parser.parseFromString(
            "<body><template>" + newContent + "</template></body>",
            "text/html"
          );
          let content = (
            /** @type {HTMLTemplateElement} */
            responseDoc.body.querySelector("template").content
          );
          generatedByIdiomorph.add(content);
          return content;
        }
      }
      return { normalizeElement: normalizeElement2, normalizeParent: normalizeParent2 };
    })();
    return {
      morph: morph2,
      defaults
    };
  })();

  // pkg/amarra/js/drive.mjs
  var drive_exports = {};
  __export(drive_exports, {
    applyDriveResponse: () => applyDriveResponse,
    driveHeaders: () => driveHeaders,
    extractMainHTML: () => extractMainHTML,
    extractMainTagName: () => extractMainTagName,
    shouldInterceptClick: () => shouldInterceptClick,
    shouldInterceptSubmit: () => shouldInterceptSubmit,
    start: () => start3,
    visit: () => visit
  });

  // pkg/amarra/js/morph.mjs
  function morph(el, html, morphFn) {
    if (!el) return;
    if (typeof morphFn === "function") return morphFn(el, html);
    const lib = globalThis.Idiomorph;
    if (lib && typeof lib.morph === "function") {
      return lib.morph(el, html, { morphStyle: "innerHTML" });
    }
    if ("innerHTML" in el) el.innerHTML = html ?? "";
  }

  // pkg/amarra/js/sequence.mjs
  var counters = /* @__PURE__ */ new WeakMap();
  var fallback = 0;
  function bumpSequence(key) {
    if (!key || typeof key !== "object") {
      fallback += 1;
      return fallback;
    }
    const next = (counters.get(key) ?? 0) + 1;
    counters.set(key, next);
    return next;
  }
  function isCurrentSequence(key, seq) {
    if (!key || typeof key !== "object") return seq === fallback;
    return seq === (counters.get(key) ?? 0);
  }

  // pkg/amarra/js/hook.mjs
  var hook_exports = {};
  __export(hook_exports, {
    afterMorph: () => afterMorph,
    applyFocus: () => applyFocus,
    applyOptimistic: () => applyOptimistic,
    csrfTokenFromMeta: () => csrfTokenFromMeta,
    dispatchLivePush: () => dispatchLivePush,
    register: () => register,
    reset: () => reset,
    rollbackOptimistic: () => rollbackOptimistic,
    scan: () => scan,
    showToast: () => showToast,
    start: () => start
  });

  // pkg/amarra/js/hook_registry.mjs
  var defs = /* @__PURE__ */ new Map();
  var mounted = /* @__PURE__ */ new Map();
  function register(name, def) {
    if (!name || !def) return;
    defs.set(name, def);
  }
  function reset() {
    defs.clear();
    mounted.clear();
  }
  function scan(root) {
    if (!root) return;
    const found = collect(root);
    const seen = new Set(found);
    for (const el of found) {
      const name = el.getAttribute?.("amarra-hook") || "";
      const def = defs.get(name);
      const cur = mounted.get(el);
      if (cur && cur.name === name) {
        def?.updated?.(el);
        continue;
      }
      if (cur) {
        cur.def.disconnect?.(el);
        mounted.delete(el);
      }
      if (!def) continue;
      mounted.set(el, { name, def });
      def.connect?.(el);
    }
    for (const [el, cur] of [...mounted]) {
      if (seen.has(el)) continue;
      cur.def.disconnect?.(el);
      mounted.delete(el);
    }
  }
  function dispatchLivePush(event, payload) {
    for (const [el, cur] of mounted) {
      cur.def.handleEvent?.(event, payload, el);
    }
  }
  function collect(root) {
    const out = [];
    if (root.hasAttribute?.("amarra-hook")) out.push(root);
    const list = root.querySelectorAll?.("[amarra-hook]");
    if (list) {
      for (const el of list) out.push(el);
    }
    return out;
  }

  // pkg/amarra/js/hook_bulk.mjs
  var STATE = "_amarraBulkState";
  function makeBulk() {
    return {
      connect(el) {
        if (!el?.querySelector) return;
        const all = el.querySelector("[data-amarra-bulk-all]");
        const rows = [...el.querySelectorAll?.("[data-amarra-bulk-row]") ?? []];
        const bar = el.querySelector("[data-amarra-bulk-bar]");
        const count = bar?.querySelector?.("[data-amarra-bulk-count]");
        if (!all || rows.length === 0) return;
        const sync = () => {
          const selected = rows.filter((r) => r.checked).length;
          all.indeterminate = selected > 0 && selected < rows.length;
          all.checked = selected === rows.length;
          if (bar) {
            bar.hidden = selected === 0;
            if (count) count.textContent = String(selected);
          }
        };
        const onAll = () => {
          for (const row of rows) row.checked = all.checked;
          sync();
        };
        all.addEventListener?.("change", onAll);
        const rowUnbinds = [];
        for (const row of rows) {
          const fn = () => sync();
          row.addEventListener?.("change", fn);
          rowUnbinds.push([row, fn]);
        }
        el[STATE] = { all, onAll, rowUnbinds };
      },
      // #126: Idiomorph keeps the container and swaps children; rebind so the
      // hook tracks the new nodes instead of stale references.
      updated(el) {
        bulk.disconnect(el);
        bulk.connect(el);
      },
      disconnect(el) {
        const st = el?.[STATE];
        if (!st) return;
        st.all.removeEventListener?.("change", st.onAll);
        for (const [row, fn] of st.rowUnbinds) row.removeEventListener?.("change", fn);
        delete el[STATE];
      }
    };
  }
  var bulk = makeBulk();

  // pkg/amarra/js/hook_clipboard.mjs
  var CLICK = "_amarraClipboardClick";
  function makeClipboard(writeText) {
    return {
      connect(el) {
        if (!el || typeof el.addEventListener !== "function") return;
        const fn = (ev) => {
          ev?.preventDefault?.();
          const text = el.getAttribute?.("data-amarra-copy") ?? "";
          writeText?.(text);
        };
        el[CLICK] = fn;
        el.addEventListener("click", fn);
      },
      disconnect(el) {
        const fn = el?.[CLICK];
        if (!fn || typeof el.removeEventListener !== "function") return;
        el.removeEventListener("click", fn);
        delete el[CLICK];
      }
    };
  }
  var clipboard = makeClipboard((text) => {
    const write = globalThis.navigator?.clipboard?.writeText;
    if (typeof write === "function") return write.call(globalThis.navigator.clipboard, text);
  });

  // pkg/amarra/js/hook_dialog.mjs
  var OPEN = "_amarraDialogOpen";
  var CLOSE = "_amarraDialogClose";
  var ONCLOSE = "_amarraDialogOnClose";
  var STATE2 = "_amarraDialogState";
  function makeDialog() {
    return {
      connect(el) {
        if (!el?.querySelector) return;
        const dlg = el.querySelector("[data-amarra-dialog-target]");
        if (!dlg || typeof dlg.showModal !== "function") return;
        if (!dlg.getAttribute?.("aria-modal")) dlg.setAttribute?.("aria-modal", "true");
        const openers = [...el.querySelectorAll?.("[data-amarra-dialog-open]") ?? []];
        const closers = [...el.querySelectorAll?.("[data-amarra-dialog-close]") ?? []];
        let opener = null;
        for (const btn of openers) {
          const fn = (ev) => {
            ev?.preventDefault?.();
            opener = btn;
            dlg.showModal?.();
          };
          btn[OPEN] = fn;
          btn.addEventListener?.("click", fn);
        }
        for (const btn of closers) {
          const fn = (ev) => {
            ev?.preventDefault?.();
            dlg.close?.();
          };
          btn[CLOSE] = fn;
          btn.addEventListener?.("click", fn);
        }
        const onClose = () => opener?.focus?.();
        dlg[ONCLOSE] = onClose;
        dlg.addEventListener?.("close", onClose);
        el[STATE2] = { dlg, openers, closers };
      },
      // #126: Idiomorph keeps the container and swaps children; rebind so the
      // hook tracks the new nodes instead of stale references.
      updated(el) {
        dialog.disconnect(el);
        dialog.connect(el);
      },
      disconnect(el) {
        const st = el?.[STATE2];
        if (!st) return;
        for (const btn of st.openers ?? []) {
          const fn = btn?.[OPEN];
          if (!fn) continue;
          btn.removeEventListener?.("click", fn);
          delete btn[OPEN];
        }
        for (const btn of st.closers ?? []) {
          const fn = btn?.[CLOSE];
          if (!fn) continue;
          btn.removeEventListener?.("click", fn);
          delete btn[CLOSE];
        }
        const onClose = st.dlg?.[ONCLOSE];
        if (onClose) {
          st.dlg.removeEventListener?.("close", onClose);
          delete st.dlg[ONCLOSE];
        }
        delete el[STATE2];
      }
    };
  }
  var dialog = makeDialog();

  // pkg/amarra/js/hook_dropdown.mjs
  var BTN = "_amarraDropdownToggle";
  var MENU = "_amarraDropdownMenuClick";
  var STATE3 = "_amarraDropdownState";
  function makeDropdown() {
    return {
      connect(el) {
        if (!el?.querySelector) return;
        const btn = el.querySelector("[data-amarra-dropdown-button]");
        const menu = el.querySelector("[data-amarra-dropdown-menu]");
        if (!btn || !menu) return;
        const doc = el.ownerDocument ?? globalThis.document;
        const close = () => {
          menu.hidden = true;
          btn.setAttribute?.("aria-expanded", "false");
        };
        const toggle = (ev) => {
          ev?.preventDefault?.();
          const open = menu.hidden;
          menu.hidden = !open;
          btn.setAttribute?.("aria-expanded", String(open));
        };
        const onDocClick = (ev) => {
          if (el.contains?.(ev?.target)) return;
          close();
        };
        const onKey = (ev) => {
          if (ev?.key === "Escape") close();
        };
        btn[BTN] = toggle;
        btn.addEventListener?.("click", toggle);
        menu[MENU] = close;
        menu.addEventListener?.("click", close);
        doc?.addEventListener?.("click", onDocClick);
        doc?.addEventListener?.("keydown", onKey);
        el[STATE3] = { btn, menu, doc, onDocClick, onKey };
      },
      // #126: Idiomorph keeps the container and swaps children; rebind so the
      // hook tracks the new nodes and document listeners do not accumulate.
      updated(el) {
        dropdown.disconnect(el);
        dropdown.connect(el);
      },
      disconnect(el) {
        const st = el?.[STATE3];
        if (!st) return;
        const toggle = st.btn?.[BTN];
        if (toggle) {
          st.btn.removeEventListener?.("click", toggle);
          delete st.btn[BTN];
        }
        const menuClose = st.menu?.[MENU];
        if (menuClose) {
          st.menu.removeEventListener?.("click", menuClose);
          delete st.menu[MENU];
        }
        if (st.onDocClick) st.doc?.removeEventListener?.("click", st.onDocClick);
        if (st.onKey) st.doc?.removeEventListener?.("keydown", st.onKey);
        delete el[STATE3];
      }
    };
  }
  var dropdown = makeDropdown();

  // pkg/amarra/js/hook_nav.mjs
  var POPSTATE = "_amarraNavPopstate";
  function makeNav(opts = {}) {
    const getLocation = opts.location ?? (() => globalThis.location);
    const getWindow = opts.window ?? globalThis.window;
    function sync(el) {
      const loc = getLocation();
      if (!el || !loc?.pathname) return;
      const on = classes(el, "data-amarra-nav-on", opts.onClasses);
      const off = classes(el, "data-amarra-nav-off", opts.offClasses);
      for (const link of el.querySelectorAll?.("a[href]") ?? []) {
        if (isActive(link, loc)) {
          off.forEach((c) => link.classList?.remove(c));
          on.forEach((c) => link.classList?.add(c));
          link.setAttribute?.("aria-current", "page");
        } else {
          on.forEach((c) => link.classList?.remove(c));
          if (off.length) off.forEach((c) => link.classList?.add(c));
          link.removeAttribute?.("aria-current");
        }
      }
    }
    return {
      connect(el) {
        if (!el) return;
        sync(el);
        const onPop = () => sync(el);
        el[POPSTATE] = onPop;
        getWindow?.addEventListener?.("popstate", onPop);
      },
      updated(el) {
        sync(el);
      },
      disconnect(el) {
        const fn = el?.[POPSTATE];
        if (!fn) return;
        getWindow?.removeEventListener?.("popstate", fn);
        delete el[POPSTATE];
      }
    };
  }
  function classes(el, attr, fallback2) {
    const raw = el.getAttribute?.(attr) || fallback2;
    if (!raw) return [];
    return raw.split(/\s+/).filter(Boolean);
  }
  function isActive(link, loc) {
    const href = link.getAttribute?.("href");
    if (!href || href.startsWith("#")) return false;
    try {
      return new URL(href, loc.href).pathname === loc.pathname;
    } catch {
      return false;
    }
  }
  var nav = makeNav();

  // pkg/amarra/js/hook_password.mjs
  var CLICK2 = "_amarraPasswordClick";
  function makePassword(findInput) {
    const resolve = findInput ?? defaultFind;
    return {
      connect(el) {
        if (!el || typeof el.addEventListener !== "function") return;
        const fn = (ev) => {
          ev?.preventDefault?.();
          const sel = el.getAttribute?.("data-amarra-password-for") ?? "";
          const input = resolve(sel, el);
          if (!input) return;
          const show = input.type === "password";
          input.type = show ? "text" : "password";
          el.setAttribute?.("aria-pressed", show ? "true" : "false");
          swapAriaLabel(el, show);
          toggleIcons(el, show);
        };
        el[CLICK2] = fn;
        el.addEventListener("click", fn);
      },
      disconnect(el) {
        const fn = el?.[CLICK2];
        if (!fn || typeof el.removeEventListener !== "function") return;
        el.removeEventListener("click", fn);
        delete el[CLICK2];
      }
    };
  }
  function defaultFind(sel, el) {
    if (sel) {
      const root = el?.ownerDocument ?? globalThis.document;
      try {
        const found = root?.querySelector?.(sel);
        if (found) return found;
      } catch {
        return null;
      }
    }
    const scope = el?.closest?.("form") ?? el?.parentElement ?? null;
    const inputs = scope?.querySelectorAll?.('input[type="password"]') ?? [];
    if (inputs.length === 0) return null;
    if (inputs.length === 1) return inputs[0];
    let best = inputs[0];
    for (const input of inputs) {
      if (el?.compareDocumentPosition?.(input) & 2) best = input;
    }
    return best;
  }
  function swapAriaLabel(el, show) {
    const showLabel = el.getAttribute?.("data-amarra-label-show");
    const hideLabel = el.getAttribute?.("data-amarra-label-hide");
    if (!showLabel && !hideLabel) return;
    el.setAttribute?.("aria-label", show ? hideLabel || showLabel : showLabel || hideLabel);
  }
  function toggleIcons(el, show) {
    const showIcon = el.querySelector?.('[data-amarra-password-icon="show"]') ?? el.querySelector?.('[data-cais-password-icon="show"]');
    const hideIcon = el.querySelector?.('[data-amarra-password-icon="hide"]') ?? el.querySelector?.('[data-cais-password-icon="hide"]');
    showIcon?.classList?.toggle?.("hidden", show);
    hideIcon?.classList?.toggle?.("hidden", !show);
  }
  var password = makePassword();

  // pkg/amarra/js/hook_reveal.mjs
  var SYNC = "_amarraRevealSync";
  function makeReveal(findTarget) {
    const resolve = findTarget ?? defaultFind2;
    return {
      connect(el) {
        if (!el || typeof el.addEventListener !== "function") return;
        const fn = () => {
          const match = el.getAttribute?.("data-amarra-reveal-show") ?? "";
          const sel = el.getAttribute?.("data-amarra-reveal-target") ?? "";
          const target = resolve(sel, el);
          if (!target) return;
          target.hidden = el.value !== match;
        };
        el[SYNC] = fn;
        el.addEventListener("change", fn);
        el.addEventListener("click", fn);
        fn();
      },
      disconnect(el) {
        const fn = el?.[SYNC];
        if (!fn || typeof el.removeEventListener !== "function") return;
        el.removeEventListener("change", fn);
        el.removeEventListener("click", fn);
        delete el[SYNC];
      }
    };
  }
  function defaultFind2(sel, el) {
    if (!sel) return null;
    const root = el?.ownerDocument ?? globalThis.document;
    try {
      return root?.querySelector?.(sel) ?? null;
    } catch {
      return null;
    }
  }
  var reveal = makeReveal();

  // pkg/amarra/js/hook_sidebar.mjs
  var OPEN_ATTR = "data-amarra-sidebar-open";
  var STATE4 = "_amarraSidebar";
  function makeSidebar(deps = {}) {
    const isDesktop = deps.isDesktop ?? (() => false);
    return {
      connect(el) {
        if (!el?.getAttribute) return;
        const doc = el.ownerDocument ?? deps.document ?? null;
        const sel = el.getAttribute("data-amarra-sidebar-target") || "";
        const panel = sel && doc?.querySelector ? doc.querySelector(sel) : null;
        if (!panel) return;
        const state = {};
        const apply = (next) => {
          state.open = next;
          if (next) panel.setAttribute?.(OPEN_ATTR, "");
          else panel.removeAttribute?.(OPEN_ATTR);
          el.setAttribute?.("aria-expanded", next ? "true" : "false");
        };
        const onToggle = () => apply(!state.open);
        const onKey = (ev) => {
          if (ev?.key !== "Escape" || !state.open) return;
          apply(false);
          el.focus?.();
        };
        const onDocClick = (ev) => {
          if (!state.open || isDesktop()) return;
          const target = ev?.target;
          if (el.contains?.(target) || panel.contains?.(target)) return;
          apply(false);
        };
        state.onToggle = onToggle;
        state.onKey = onKey;
        state.onDocClick = onDocClick;
        el.addEventListener?.("click", onToggle);
        doc?.addEventListener?.("keydown", onKey);
        doc?.addEventListener?.("click", onDocClick, true);
        apply(false);
        el[STATE4] = state;
      },
      disconnect(el) {
        const state = el?.[STATE4];
        if (!state) return;
        el.removeEventListener?.("click", state.onToggle);
        const doc = el.ownerDocument ?? deps.document ?? null;
        doc?.removeEventListener?.("keydown", state.onKey);
        doc?.removeEventListener?.("click", state.onDocClick, true);
        delete el[STATE4];
      }
    };
  }
  var sidebar = makeSidebar();

  // pkg/amarra/js/hook_theme.mjs
  var CLICK3 = "_amarraThemeClick";
  var DEFAULT_KEY = "amarra-theme";
  var DEFAULT_CLASS = "light";
  function makeTheme(opts = {}) {
    const getHtml = opts.html ?? (() => globalThis.document?.documentElement);
    const getStorage = () => opts.storage ?? globalThis.localStorage;
    const getMeta = opts.themeColorMeta ?? (() => globalThis.document?.querySelector?.('meta[name="theme-color"]'));
    const keyFor = (el) => el?.getAttribute?.("data-amarra-theme-key") || opts.key || DEFAULT_KEY;
    const classFor = (el) => el?.getAttribute?.("data-amarra-theme-class") || opts.className || DEFAULT_CLASS;
    function apply(on, el) {
      const className = classFor(el);
      const key = keyFor(el);
      const html = getHtml();
      if (html?.classList) {
        if (on) html.classList.add(className);
        else html.classList.remove(className);
      }
      try {
        getStorage()?.setItem?.(key, on ? className : "");
      } catch {
      }
      const meta = getMeta?.();
      const lightColor = el?.getAttribute?.("data-amarra-theme-color") || opts.lightColor;
      const darkColor = el?.getAttribute?.("data-amarra-theme-color-off") || opts.darkColor;
      const color = on ? lightColor : darkColor;
      if (meta && color) meta.setAttribute?.("content", color);
      const onLabel = el?.getAttribute?.("data-amarra-theme-on-label") || opts.onLabel;
      const offLabel = el?.getAttribute?.("data-amarra-theme-off-label") || opts.offLabel;
      const label = on ? onLabel : offLabel;
      if (label && el) swapThemeLabel(el, label);
      el?.setAttribute?.("aria-pressed", on ? "true" : "false");
    }
    return {
      connect(el) {
        if (!el || typeof el.addEventListener !== "function") return;
        let stored = "";
        try {
          stored = getStorage()?.getItem?.(keyFor(el)) ?? "";
        } catch {
          stored = "";
        }
        if (stored === classFor(el)) apply(true, el);
        const fn = (ev) => {
          ev?.preventDefault?.();
          const html = getHtml();
          const on = !html?.classList?.contains?.(classFor(el));
          apply(on, el);
        };
        el[CLICK3] = fn;
        el.addEventListener("click", fn);
      },
      disconnect(el) {
        const fn = el?.[CLICK3];
        if (!fn || typeof el.removeEventListener !== "function") return;
        el.removeEventListener("click", fn);
        delete el[CLICK3];
      }
    };
  }
  var theme = makeTheme();
  function swapThemeLabel(el, label) {
    const slot = el.querySelector?.("[data-amarra-theme-label]");
    if (slot) {
      slot.textContent = label;
      return;
    }
    if (el.children?.length) return;
    el.textContent = label;
  }

  // pkg/amarra/js/hook.mjs
  register("bulk", bulk);
  register("clipboard", clipboard);
  register("dialog", dialog);
  register("dropdown", dropdown);
  register("nav", nav);
  register("password", password);
  register("reveal", reveal);
  register("sidebar", sidebar);
  register("theme", theme);
  var ON_CLASSES = ["bg-green-50", "text-green-700"];
  var OFF_CLASSES = ["bg-slate-100", "text-slate-600"];
  var TOAST_MS = 2e3;
  function csrfTokenFromMeta(htmlOrDoc) {
    if (!htmlOrDoc) return "";
    if (typeof htmlOrDoc === "string") {
      const named = htmlOrDoc.match(/<meta\b[^>]*\bname\s*=\s*["']csrf-token["'][^>]*>/i);
      const tag = named?.[0] || htmlOrDoc.match(
        /<meta\b[^>]*\bcontent\s*=\s*["'][^"']*["'][^>]*\bname\s*=\s*["']csrf-token["'][^>]*>/i
      )?.[0];
      if (!tag) return "";
      const content = tag.match(/\bcontent\s*=\s*["']([^"']*)["']/i);
      return content ? content[1] : "";
    }
    const meta = htmlOrDoc.querySelector?.('meta[name="csrf-token"]');
    if (!meta) return "";
    return meta.content || meta.getAttribute?.("content") || "";
  }
  function showToast(message, doc, opts = {}) {
    if (!message || !doc) return;
    const host = doc.getElementById?.("amarra-toast-host");
    if (!host) return;
    if (host._amarraToastTimer) {
      clearTimeout(host._amarraToastTimer);
      host._amarraToastTimer = null;
    }
    host.innerHTML = '<div class="amarra-toast-enter fixed top-24 left-1/2 -translate-x-1/2 z-50 bg-slate-900 text-white px-5 py-3 rounded-2xl shadow-xl flex items-center gap-2 border border-slate-700/50" role="status"><span class="text-xs font-bold"></span></div>';
    const span = host.querySelector?.("span");
    if (span) span.textContent = message;
    const duration = opts.duration ?? TOAST_MS;
    if (duration > 0) {
      host._amarraToastTimer = setTimeout(() => {
        host.innerHTML = "";
        host._amarraToastTimer = null;
      }, duration);
    }
  }
  function applyFocus(selector, doc) {
    if (!selector || !doc?.querySelector) return;
    const el = doc.querySelector(selector);
    if (el && typeof el.focus === "function") el.focus();
  }
  function afterMorph(doc) {
    if (!doc?.querySelector) return;
    const marked = doc.querySelector("[data-amarra-focus]");
    if (!marked) return;
    const sel = marked.getAttribute?.("data-amarra-focus");
    if (sel && sel !== "true") {
      applyFocus(sel, doc);
      return;
    }
    if (typeof marked.focus === "function") marked.focus();
  }
  function applyOptimistic(el, mode) {
    if (!el) return null;
    if (mode === "count") {
      const countEl = el.querySelector?.("[data-amarra-count]") || el;
      const prev = countEl.textContent;
      const n = parseInt(String(prev ?? "").trim(), 10);
      countEl.textContent = String((Number.isNaN(n) ? 0 : n) + 1);
      return { el, mode, countEl, prev };
    }
    if (mode === "remove") {
      const hadOpacity = !!el.classList?.contains("opacity-0");
      el.classList?.add("opacity-0", "transition-opacity", "duration-150");
      return { el, mode, hadOpacity };
    }
    const wasOn = hasClasses(el, ON_CLASSES);
    if (wasOn) setClasses(el, OFF_CLASSES, ON_CLASSES);
    else setClasses(el, ON_CLASSES, OFF_CLASSES);
    return { el, mode: "toggle", wasOn };
  }
  function rollbackOptimistic(state) {
    if (!state?.el) return;
    const { el } = state;
    if (state.mode === "count") {
      state.countEl.textContent = state.prev;
      return;
    }
    if (state.mode === "remove") {
      if (!state.hadOpacity) el.classList?.remove("opacity-0", "transition-opacity", "duration-150");
      return;
    }
    if (state.wasOn) setClasses(el, ON_CLASSES, OFF_CLASSES);
    else setClasses(el, OFF_CLASSES, ON_CLASSES);
  }
  function start(opts = {}) {
    const doc = opts.document ?? (typeof document !== "undefined" ? document : null);
    if (!doc || typeof doc.addEventListener !== "function") return;
    if (doc.documentElement?.dataset?.amarraHook === "true") return;
    if (doc.documentElement?.dataset) doc.documentElement.dataset.amarraHook = "true";
    register("bulk", bulk);
    register("clipboard", clipboard);
    register("dialog", dialog);
    register("dropdown", dropdown);
    register("nav", nav);
    register("password", password);
    register("reveal", reveal);
    register("sidebar", sidebar);
    register("theme", theme);
    scan(doc);
    let optimistic = null;
    doc.addEventListener("amarra:toast", (ev) => {
      showToast(ev.detail?.message ?? "", doc, opts);
    });
    doc.addEventListener("amarra:morphed", () => {
      optimistic = null;
      afterMorph(doc);
      scan(doc);
    });
    doc.addEventListener("amarra:drive-error", () => {
      rollbackOptimistic(optimistic);
      optimistic = null;
    });
    doc.addEventListener(
      "click",
      (ev) => {
        const target = ev.target?.closest?.("[data-amarra-optimistic]");
        if (!target) return;
        optimistic = applyOptimistic(target, target.getAttribute("data-amarra-optimistic"));
        target.setAttribute?.("aria-busy", "true");
      },
      true
    );
  }
  function hasClasses(el, classes2) {
    return classes2.every((c) => el.classList?.contains(c));
  }
  function setClasses(el, add, remove) {
    remove.forEach((c) => el.classList?.remove(c));
    add.forEach((c) => el.classList?.add(c));
  }

  // pkg/amarra/js/frame.mjs
  var frame_exports = {};
  __export(frame_exports, {
    define: () => define,
    frameHeaders: () => frameHeaders,
    frameTarget: () => frameTarget,
    loadFrame: () => loadFrame,
    observeLazy: () => observeLazy,
    visitIntoFrame: () => visitIntoFrame
  });
  function frameHeaders(id, csrfToken) {
    const headers = {
      "Amarra-Frame": String(id ?? ""),
      Accept: "text/html"
    };
    if (csrfToken) headers["X-CSRF-Token"] = csrfToken;
    return headers;
  }
  function frameTarget(el) {
    return el?.getAttribute?.("data-amarra-frame") || "";
  }
  async function visitIntoFrame(el, href, opts = {}) {
    const id = frameTarget(el);
    if (!id || id === "_top") return false;
    const doc = opts.document ?? (typeof document !== "undefined" ? document : null);
    const frame = doc?.getElementById?.(id) || null;
    if (!frame) return false;
    const prev = frame.getAttribute?.("src");
    if (typeof frame.setAttribute === "function") frame.setAttribute("src", href);
    const tag = frame.tagName ? String(frame.tagName).toUpperCase() : "";
    if (tag === "AMARRA-FRAME" && prev !== href) return true;
    await loadFrame(frame, { ...opts, document: doc });
    return true;
  }
  function observeLazy(el, opts = {}) {
    const IO = opts.IntersectionObserver ?? globalThis.IntersectionObserver;
    if (typeof IO !== "function") {
      return loadFrame(el, opts);
    }
    const io = new IO((entries) => {
      if (!entries?.some?.((e) => e.isIntersecting)) return;
      io.disconnect();
      void loadFrame(el, opts);
    });
    io.observe(el);
    return io;
  }
  async function loadFrame(el, opts = {}) {
    if (!el) return;
    const src = el.getAttribute?.("src");
    if (!src) return;
    const id = el.getAttribute?.("id") || el.id || "";
    const fetchFn = opts.fetchFn ?? opts.fetch ?? fetch;
    const csrfToken = opts.csrfToken ?? csrfTokenFromMeta(opts.document ?? (typeof document !== "undefined" ? document : ""));
    const seq = bumpSequence(el);
    const res = await fetchFn(src, {
      headers: frameHeaders(id, csrfToken),
      credentials: "same-origin",
      redirect: "follow"
    });
    const html = await res.text();
    if (!isCurrentSequence(el, seq)) return;
    if (el.isConnected === false) return;
    (opts.morphFn ?? morph)(el, html);
    const doc = opts.document ?? (typeof document !== "undefined" ? document : null);
    if (doc && typeof doc.dispatchEvent === "function") {
      doc.dispatchEvent(new CustomEvent("amarra:morphed", { bubbles: true }));
    }
    if (el.hasAttribute?.("amarra-push") && opts.history?.pushState) {
      opts.history.pushState({ amarra: true }, "", src);
    }
  }
  function define(opts = {}) {
    const registry = opts.customElements ?? (typeof customElements !== "undefined" ? customElements : null);
    const Base = opts.HTMLElement ?? (typeof HTMLElement !== "undefined" ? HTMLElement : null);
    if (!registry || !Base || typeof registry.define !== "function") return false;
    if (typeof registry.get === "function" && registry.get("amarra-frame")) return true;
    class AmarraFrame extends Base {
      connectedCallback() {
        if (!this.getAttribute("src")) return;
        if (this.getAttribute("loading") === "lazy") {
          observeLazy(this, opts);
          return;
        }
        loadFrame(this, opts);
      }
      static get observedAttributes() {
        return ["src"];
      }
      attributeChangedCallback(name, prev, next) {
        if (name === "src" && next && prev !== next) loadFrame(this, opts);
      }
    }
    registry.define("amarra-frame", AmarraFrame);
    return true;
  }

  // pkg/amarra/js/stream.mjs
  var stream_exports = {};
  __export(stream_exports, {
    applyOp: () => applyOp,
    connect: () => connect,
    isStreamResponse: () => isStreamResponse,
    parseSSE: () => parseSSE,
    start: () => start2
  });
  var STREAM_KINDS = [
    "append",
    "prepend",
    "replace",
    "morph",
    "remove",
    "toast",
    "before",
    "after"
  ];
  function parseSSE(chunk) {
    const events = [];
    const text = String(chunk ?? "").replace(/\r\n?/g, "\n");
    for (const block of text.split("\n\n")) {
      if (!block.trim()) continue;
      let kind = "message";
      let target;
      const dataLines = [];
      for (const line of block.split("\n")) {
        if (!line || line.startsWith(":")) continue;
        if (line.startsWith("event:")) {
          kind = line.slice(6).trim();
          continue;
        }
        if (line.startsWith("id:")) {
          const rest = line.slice(3);
          target = rest.startsWith(" ") ? rest.slice(1) : rest;
          continue;
        }
        if (line.startsWith("data:")) {
          const rest = line.slice(5);
          dataLines.push(rest.startsWith(" ") ? rest.slice(1) : rest);
        }
      }
      const op = { kind, html: dataLines.join("\n") };
      if (target) op.target = target;
      events.push(op);
    }
    return events;
  }
  function applyOp(op, doc, opts = {}) {
    if (!op) return;
    if (op.kind === "toast") {
      if (doc && typeof doc.dispatchEvent === "function") {
        doc.dispatchEvent(
          new CustomEvent("amarra:toast", { bubbles: true, detail: { message: op.html ?? "" } })
        );
      }
      return;
    }
    const id = op.target || opts.defaultTarget;
    const el = id && doc?.getElementById ? doc.getElementById(id) : null;
    if (!el) return;
    const html = op.html ?? "";
    switch (op.kind) {
      case "append":
        insertHTML(el, "beforeend", html);
        break;
      case "prepend":
        insertHTML(el, "afterbegin", html);
        break;
      case "before":
        insertHTML(el, "beforebegin", html);
        break;
      case "after":
        insertHTML(el, "afterend", html);
        break;
      case "replace":
        el.outerHTML = html;
        break;
      case "morph":
        (opts.morphFn ?? morph)(el, html);
        break;
      case "remove":
        el.remove?.();
        break;
    }
  }
  function connect(url, opts = {}) {
    const ES = opts.EventSource ?? (typeof EventSource !== "undefined" ? EventSource : null);
    if (!ES || !url) return null;
    const src = new ES(url);
    const doc = opts.document ?? (typeof document !== "undefined" ? document : null);
    for (const kind of STREAM_KINDS) {
      src.addEventListener(kind, (ev) => {
        for (const op of parseSSE(sseEnvelope(kind, ev.data))) {
          applyOp(op, doc, opts);
        }
      });
    }
    return src;
  }
  function start2(opts = {}) {
    const doc = opts.document ?? (typeof document !== "undefined" ? document : null);
    if (!doc || typeof doc.addEventListener !== "function") return;
    if (doc.documentElement?.dataset?.amarraStream === "true") return;
    if (doc.documentElement?.dataset) doc.documentElement.dataset.amarraStream = "true";
    const sources = /* @__PURE__ */ new Map();
    const sync = () => {
      for (const [el, src] of sources) {
        if (el.isConnected === false) {
          src?.close?.();
          sources.delete(el);
        }
      }
      const nodes = typeof doc.querySelectorAll === "function" ? doc.querySelectorAll("[data-amarra-stream]") : [];
      for (const el of nodes) {
        if (el === doc.documentElement) continue;
        if (sources.has(el)) continue;
        const url = el.getAttribute?.("data-amarra-stream");
        if (!url) continue;
        const src = connect(url, {
          ...opts,
          document: doc,
          defaultTarget: el.getAttribute?.("data-amarra-target") || opts.defaultTarget
        });
        if (src) sources.set(el, src);
      }
    };
    sync();
    doc.addEventListener("amarra:morphed", sync);
  }
  function sseEnvelope(kind, data) {
    const lines = String(data ?? "").split("\n").map((line) => `data: ${line}`);
    return `event: ${kind}
${lines.join("\n")}

`;
  }
  function isStreamResponse(headers) {
    if (!headers) return false;
    const ct = typeof headers.get === "function" ? headers.get("content-type") : headers["content-type"] || headers["Content-Type"];
    return String(ct || "").includes("vnd.amarra-stream");
  }
  function insertHTML(el, pos, html) {
    if (typeof el.insertAdjacentHTML === "function") {
      el.insertAdjacentHTML(pos, html);
      return;
    }
    if (pos === "afterbegin") el.innerHTML = html + (el.innerHTML || "");
    else el.innerHTML = (el.innerHTML || "") + html;
  }

  // pkg/amarra/js/drive_head.mjs
  function extractTitle(html) {
    const m = String(html ?? "").match(/<title\b[^>]*>([\s\S]*?)<\/title>/i);
    return m ? m[1].trim() : null;
  }
  function extractHTMLAttr(html, name) {
    const open = String(html ?? "").match(/<html\b[^>]*>/i)?.[0] ?? "";
    const escaped = String(name).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const m = open.match(new RegExp(`\\s${escaped}\\s*=\\s*["']([^"']*)["']`, "i"));
    return m ? m[1] : null;
  }
  function applyHead(doc, html) {
    if (!doc) return;
    const title = extractTitle(html);
    if (title != null) doc.title = title;
    const lang = extractHTMLAttr(html, "lang");
    if (lang != null && doc.documentElement) doc.documentElement.lang = lang;
    const token = csrfTokenFromMeta(html);
    if (!token) return;
    const meta = doc.querySelector?.('meta[name="csrf-token"]');
    if (!meta) return;
    if (typeof meta.setAttribute === "function") meta.setAttribute("content", token);
    else meta.content = token;
  }
  function ensureVeilKeyframes(doc) {
    if (!doc?.createElement || doc.getElementById?.("amarra-veil-style")) return;
    const style = doc.createElement("style");
    style.id = "amarra-veil-style";
    style.textContent = "@keyframes amarra-spin{to{transform:rotate(360deg)}}@media (prefers-reduced-motion:reduce){#amarra-veil div{animation:none!important}}";
    (doc.head ?? doc.body)?.appendChild?.(style);
  }
  function showProgress(doc) {
    if (!doc?.createElement || !doc.body) return;
    let bar = doc.getElementById?.("amarra-progress");
    if (!bar) {
      bar = doc.createElement("div");
      bar.id = "amarra-progress";
      bar.setAttribute("role", "progressbar");
      bar.style.cssText = "position:fixed;top:0;left:0;right:0;height:2px;background:#c9893a;z-index:9999";
      doc.body.appendChild(bar);
    }
    bar.hidden = false;
    let veil = doc.getElementById?.("amarra-veil");
    if (!veil) {
      veil = doc.createElement("div");
      veil.id = "amarra-veil";
      veil.setAttribute("role", "status");
      veil.setAttribute("aria-label", "Loading");
      veil.style.cssText = "position:fixed;inset:0;z-index:9998;display:flex;align-items:center;justify-content:center;background:rgba(10,10,12,.45);backdrop-filter:blur(2px);opacity:0;transition:opacity .18s ease;pointer-events:none";
      const spinner = doc.createElement("div");
      spinner.setAttribute("role", "presentation");
      spinner.style.cssText = "width:52px;height:52px;border-radius:50%;border:4px solid rgba(201,137,58,.25);border-top-color:#c9893a;animation:amarra-spin .9s linear infinite";
      veil.appendChild?.(spinner);
      ensureVeilKeyframes(doc);
      doc.body.appendChild(veil);
    }
    veil.hidden = false;
    if (veil.style) veil.style.opacity = "1";
  }
  function hideProgress(doc) {
    const bar = doc?.getElementById?.("amarra-progress");
    if (bar) bar.hidden = true;
    const veil = doc?.getElementById?.("amarra-veil");
    if (veil) {
      if (veil.style) veil.style.opacity = "0";
      veil.hidden = true;
    }
  }

  // pkg/amarra/js/drive_form.mjs
  function confirmOk(el, confirmFn) {
    const msg = el?.getAttribute?.("data-amarra-confirm");
    if (!msg) return true;
    const fn = confirmFn ?? (typeof globalThis.confirm === "function" ? globalThis.confirm.bind(globalThis) : () => true);
    return !!fn(msg);
  }
  function requestMethod(el, fallback2 = "GET") {
    const attr = el?.getAttribute?.("data-amarra-method");
    if (attr) return String(attr).toUpperCase();
    const hidden = el?.querySelector?.('input[name="_method"]');
    if (hidden?.value) return String(hidden.value).toUpperCase();
    return String(fallback2 || "GET").toUpperCase();
  }
  function disableSubmit(el) {
    if (!el) return null;
    const prev = { el, disabled: !!el.disabled, text: el.textContent };
    el.disabled = true;
    const withText = el.getAttribute?.("data-amarra-disable-with");
    if (withText) el.textContent = withText;
    return prev;
  }
  function restoreSubmit(prev) {
    if (!prev?.el) return;
    prev.el.disabled = prev.disabled;
    if (prev.text != null) prev.el.textContent = prev.text;
  }
  function driveFormBody(formData, URLSearchParamsCtor = URLSearchParams, FileCtor = typeof File !== "undefined" ? File : null) {
    if (!formData) return null;
    for (const [, value] of formData.entries()) {
      if (FileCtor && value instanceof FileCtor) return formData;
    }
    const body = new URLSearchParamsCtor();
    for (const [key, value] of formData.entries()) body.append(key, value);
    return body;
  }

  // pkg/amarra/js/drive_restore.mjs
  function captureScroll(history, y) {
    if (!history?.replaceState) return;
    const prev = history.state && typeof history.state === "object" ? history.state : {};
    history.replaceState({ ...prev, amarra: true, scrollY: y ?? 0 }, "");
  }
  function restoreScroll(win, state) {
    const y = state?.scrollY;
    if (typeof y !== "number") return;
    win?.scrollTo?.(0, y);
  }
  function focusFirstInvalid(doc) {
    const el = doc?.querySelector?.('[aria-invalid="true"], [data-amarra-invalid]');
    if (el && typeof el.focus === "function") el.focus();
  }

  // pkg/amarra/js/drive.mjs
  function shouldInterceptClick({
    href,
    target,
    download,
    origin,
    locationOrigin,
    skip,
    currentHref,
    resolvedHref,
    button
  } = {}) {
    if (skip) return false;
    if (button != null && button !== 0) return false;
    if (download) return false;
    if (target && target !== "_self") return false;
    if (!href) return false;
    if (/^(mailto|javascript|tel):/i.test(href)) return false;
    if (isHashOnlyNavigation(href, resolvedHref, currentHref)) return false;
    if (origin && locationOrigin && origin !== locationOrigin) return false;
    return true;
  }
  function shouldInterceptSubmit({ skip, target, method, origin, locationOrigin } = {}) {
    if (skip) return false;
    if (target && target !== "_self") return false;
    if (String(method || "GET").toLowerCase() === "dialog") return false;
    if (origin && locationOrigin && origin !== locationOrigin) return false;
    return true;
  }
  function driveHeaders(csrfToken) {
    const headers = {
      "Amarra-Drive": "true",
      Accept: "text/html"
    };
    if (csrfToken) headers["X-CSRF-Token"] = csrfToken;
    return headers;
  }
  function extractMainHTML(html) {
    const str = String(html ?? "");
    const open = extractMainOpen(str);
    if (!open) return null;
    const start5 = open.index + open[0].length;
    return sliceMatchingClose(str, start5, open[1]);
  }
  function extractMainTagName(html) {
    return extractMainOpen(String(html ?? ""))?.[1]?.toUpperCase() ?? null;
  }
  function defaultDriveWarn(msg) {
    try {
      globalThis.console?.warn?.(msg);
    } catch {
    }
  }
  function applyDriveResponse({
    status,
    html,
    url,
    main,
    morphFn,
    location,
    history,
    document: doc,
    push = true,
    window: win,
    warn = defaultDriveWarn
  } = {}) {
    if (status === 401 || status === 403) {
      location?.reload?.();
      return { action: "reload" };
    }
    if (status !== 200 && status !== 422) {
      emitDriveError(doc);
      return { action: "ignore" };
    }
    const fragment = extractMainHTML(html);
    if (fragment == null) {
      warn(
        `amarra drive: ignored ${status} response for ${url ?? "(unknown url)"} \u2014 #amarra-main missing or unbalanced HTML`
      );
      return { action: "ignore" };
    }
    if (needsFullVisit({ html, main, document: doc })) {
      assignLocation(location, url);
      return { action: "assign" };
    }
    applyHead(doc, html);
    applyLayoutMarker(doc, html);
    if (main) (morphFn ?? morph)(main, fragment);
    if (status === 200 && push && url && history?.pushState) {
      if (!location?.href || url !== location.href) {
        history.pushState({ amarra: true, scrollY: 0 }, "", url);
      }
      win?.scrollTo?.(0, 0);
    }
    if (status === 200 && !push) restoreScroll(win, history?.state);
    if (status === 422) focusFirstInvalid(doc);
    if (doc && typeof doc.dispatchEvent === "function") {
      doc.dispatchEvent(new CustomEvent("amarra:morphed", { bubbles: true }));
    }
    return { action: "morph" };
  }
  function needsFullVisit({ html, main, document: doc }) {
    const currentLayout = doc?.documentElement?.dataset?.amarraLayout;
    const nextLayout = extractHTMLAttr(html, "data-amarra-layout");
    if (currentLayout && nextLayout && currentLayout !== nextLayout) return true;
    const currentTag = main?.tagName?.toUpperCase?.();
    const nextTag = extractMainTagName(html);
    return !!(currentTag && nextTag && currentTag !== nextTag);
  }
  function assignLocation(location, url) {
    if (url && typeof location?.assign === "function") {
      location.assign(url);
      return;
    }
    if (url && location) location.href = url;
  }
  function applyLayoutMarker(doc, html) {
    const layout = extractHTMLAttr(html, "data-amarra-layout");
    if (layout != null && doc?.documentElement?.dataset) {
      doc.documentElement.dataset.amarraLayout = layout;
    }
  }
  async function visit(url, opts = {}) {
    const fetchFn = opts.fetchFn ?? opts.fetch ?? fetch;
    const doc = opts.document;
    const seq = bumpSequence(doc);
    showProgress(doc);
    try {
      const res = await fetchFn(url, {
        method: opts.method ?? "GET",
        headers: { ...driveHeaders(opts.csrfToken), ...opts.headers },
        body: opts.body,
        redirect: "follow",
        credentials: "same-origin"
      });
      if (!isCurrentSequence(doc, seq)) {
        return { action: "superseded" };
      }
      const win = opts.window ?? (typeof window !== "undefined" ? window : null);
      const location = opts.location ?? win?.location ?? null;
      const history = opts.history ?? win?.history ?? null;
      if (opts.push !== false) captureScroll(history, win?.scrollY ?? 0);
      const html = res.status === 401 || res.status === 403 ? "" : await res.text();
      if (isStreamResponse(res.headers)) {
        for (const op of parseSSE(html)) applyOp(op, doc, opts);
        return { action: "stream" };
      }
      return applyDriveResponse({
        status: res.status,
        html,
        url: res.url || url,
        main: doc?.querySelector?.("#amarra-main") ?? opts.main ?? null,
        morphFn: opts.morphFn,
        location,
        history,
        document: doc,
        push: opts.push !== false,
        window: win
      });
    } finally {
      if (isCurrentSequence(doc, seq)) hideProgress(doc);
    }
  }
  function start3(opts = {}) {
    const doc = opts.document ?? (typeof document !== "undefined" ? document : null);
    if (!doc || typeof doc.addEventListener !== "function") return;
    if (doc.documentElement?.dataset?.amarraDrive === "true") return;
    if (doc.documentElement?.dataset) doc.documentElement.dataset.amarraDrive = "true";
    const location = opts.location ?? (typeof window !== "undefined" ? window.location : null);
    const history = opts.history ?? (typeof window !== "undefined" ? window.history : null);
    const fetchFn = opts.fetchFn ?? opts.fetch ?? (typeof fetch !== "undefined" ? fetch : null);
    const csrfToken = opts.csrfToken ?? csrfTokenFromMeta(doc);
    const FormDataCtor = opts.FormData ?? (typeof FormData !== "undefined" ? FormData : null);
    const shared = { ...opts, document: doc, location, history, fetchFn, csrfToken };
    doc.addEventListener("click", (event) => {
      if (event.defaultPrevented) return;
      if (event.button != null && event.button !== 0) return;
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const a = findAnchor(event.target);
      if (!a) return;
      const href = a.getAttribute?.("href") ?? a.href;
      const resolved = resolveURL(href, location);
      if (!shouldInterceptClick({
        href,
        resolvedHref: resolved?.href,
        currentHref: location?.href,
        target: a.getAttribute?.("target") ?? a.target ?? "",
        download: !!(a.hasAttribute?.("download") || a.download),
        origin: resolved?.origin,
        locationOrigin: location?.origin,
        skip: a.hasAttribute?.("data-amarra-skip"),
        button: event.button
      })) {
        return;
      }
      if (!confirmOk(a, opts.confirm)) return;
      const method = requestMethod(a, "GET");
      event.preventDefault();
      const hrefURL = resolved?.href ?? href;
      void (async () => {
        if (await visitIntoFrame(a, hrefURL, shared)) return;
        await visit(hrefURL, { ...shared, method });
      })().catch(() => emitDriveError(doc));
    });
    doc.addEventListener("submit", (event) => {
      if (event.defaultPrevented) return;
      const form = findForm(event.target);
      if (!form) return;
      const submitter = event.submitter;
      const rawAction = submitter?.getAttribute?.("formaction") || form.getAttribute?.("action") || form.action || location?.href || "";
      const method = (submitter?.getAttribute?.("formmethod") || form.getAttribute?.("method") || form.method || "GET").toUpperCase();
      const resolved = resolveURL(rawAction, location);
      if (!shouldInterceptSubmit({
        skip: form.hasAttribute?.("data-amarra-skip"),
        target: form.getAttribute?.("target") ?? form.target ?? "",
        method,
        origin: resolved?.origin,
        locationOrigin: location?.origin
      })) {
        return;
      }
      if (!confirmOk(form, opts.confirm) || !confirmOk(submitter, opts.confirm)) return;
      const verb = requestMethod(form, method);
      event.preventDefault();
      const fd = FormDataCtor ? formDataWithSubmitter(form, submitter, FormDataCtor) : null;
      const url = verb === "GET" ? withQuery(rawAction, fd) : rawAction;
      const body = verb === "GET" ? void 0 : driveFormBody(fd);
      const disabled = disableSubmit(submitter);
      void visit(url, {
        ...shared,
        method: verb,
        body
      }).catch(() => emitDriveError(doc)).finally(() => restoreSubmit(disabled));
    });
    if (typeof window !== "undefined" && opts.popstate !== false) {
      window.addEventListener("popstate", () => {
        void visit(location?.href ?? window.location.href, { ...shared, push: false });
      });
    }
  }
  function isHashOnlyNavigation(href, resolvedHref, currentHref) {
    if (href === "#" || typeof href === "string" && href.startsWith("#")) return true;
    if (!currentHref) return false;
    try {
      const next = new URL(resolvedHref || href, currentHref);
      const cur = new URL(currentHref);
      if (next.origin !== cur.origin || next.pathname !== cur.pathname || next.search !== cur.search) {
        return false;
      }
      return next.hash !== cur.hash;
    } catch {
      return false;
    }
  }
  function sliceMatchingClose(str, start5, tag) {
    const lower = str.toLowerCase();
    const name = tag.toLowerCase();
    const closeToken = `</${name}>`;
    let depth = 1;
    let i = start5;
    while (i < str.length && depth > 0) {
      const nextOpen = findOpenTag(lower, i, name);
      const nextClose = lower.indexOf(closeToken, i);
      if (nextClose === -1) return null;
      if (nextOpen !== -1 && nextOpen < nextClose) {
        depth += 1;
        i = nextOpen + name.length + 1;
        continue;
      }
      depth -= 1;
      if (depth === 0) return str.slice(start5, nextClose);
      i = nextClose + closeToken.length;
    }
    return null;
  }
  function extractMainOpen(str) {
    return str.match(/<([a-zA-Z][\w:-]*)(?=[^>]*\sid\s*=\s*["']amarra-main["'])[^>]*>/i);
  }
  function findOpenTag(lower, from, name) {
    const token = `<${name}`;
    let i = from;
    while (i < lower.length) {
      const j = lower.indexOf(token, i);
      if (j === -1) return -1;
      const after = lower[j + token.length];
      if (after === ">" || after === "/" || after && /\s/.test(after)) return j;
      i = j + token.length;
    }
    return -1;
  }
  function formDataWithSubmitter(form, submitter, Ctor) {
    try {
      return new Ctor(form, submitter);
    } catch {
      const fd = new Ctor();
      if (submitter?.name && typeof fd.append === "function") {
        fd.append(submitter.name, submitter.value ?? "");
      }
      return fd;
    }
  }
  function emitDriveError(doc) {
    if (doc && typeof doc.dispatchEvent === "function") {
      doc.dispatchEvent(new CustomEvent("amarra:drive-error", { bubbles: true }));
    }
  }
  function findAnchor(target) {
    if (!target) return null;
    if (typeof target.closest === "function") return target.closest("a[href]");
    let node = target;
    while (node) {
      const tag = node.tagName;
      if ((tag === "A" || tag === "a") && (node.href || node.getAttribute?.("href"))) return node;
      node = node.parentElement || node.parentNode;
    }
    return null;
  }
  function findForm(target) {
    if (!target) return null;
    if (target.tagName === "FORM" || target.tagName === "form") return target;
    if (typeof target.closest === "function") return target.closest("form");
    return null;
  }
  function resolveURL(href, location) {
    if (href == null || href === "") {
      try {
        return location?.href ? new URL(location.href) : null;
      } catch {
        return null;
      }
    }
    try {
      return new URL(href, location?.href ?? location?.origin ?? "http://localhost");
    } catch {
      return null;
    }
  }
  function withQuery(action, fd) {
    if (!fd || typeof fd.entries !== "function") return action;
    const params = new URLSearchParams();
    for (const [key, value] of fd.entries()) {
      if (typeof value === "string") params.append(key, value);
    }
    const q = params.toString();
    if (!q) return action;
    return action.includes("?") ? `${action}&${q}` : `${action}?${q}`;
  }

  // pkg/amarra/js/live.mjs
  var live_exports = {};
  __export(live_exports, {
    applyLiveMessage: () => applyLiveMessage,
    debounceWait: () => debounceWait,
    eventName: () => eventName,
    formPayload: () => formPayload,
    liveRoot: () => liveRoot,
    setLoading: () => setLoading,
    start: () => start4,
    wsURL: () => wsURL
  });
  function wsURL(loc, { view, topic } = {}) {
    if (!loc?.host) return "";
    const proto = loc.protocol === "https:" ? "wss:" : "ws:";
    const q = new URLSearchParams();
    if (view) q.set("view", view);
    if (topic) q.set("topic", topic);
    const qs = q.toString();
    return `${proto}//${loc.host}/amarra/live${qs ? `?${qs}` : ""}`;
  }
  function liveRoot(el) {
    return el?.closest?.("[amarra-live]") ?? null;
  }
  function eventName(el, kind) {
    if (!el?.getAttribute) return "";
    if (kind === "click") return el.getAttribute("amarra-click") || "";
    if (kind === "change") return el.getAttribute("amarra-change") || "";
    if (kind === "submit") return el.getAttribute("amarra-submit") || "";
    return "";
  }
  function formPayload(form, FormDataCtor = FormData) {
    if (!form || !FormDataCtor) return {};
    const data = {};
    for (const [k, v] of new FormDataCtor(form)) {
      data[k] = v;
    }
    return data;
  }
  function debounceWait(el) {
    const n = parseInt(el?.getAttribute?.("amarra-debounce") || "0", 10);
    return Number.isFinite(n) && n > 0 ? n : 0;
  }
  function setLoading(el, root, on) {
    const op = on ? "add" : "remove";
    el?.classList?.[op]?.("amarra-click-loading");
    root?.classList?.[op]?.("amarra-loading");
  }
  function applyLiveMessage(msg, root, morphFn = morph, extras = {}) {
    if (!msg || !root) return;
    if (msg.type !== "ok" && msg.type !== "morph") return;
    const html = msg.html ?? "";
    if (html !== "") {
      if (msg.target) {
        const el = root.querySelector?.(`#${cssEscape(msg.target)}`) || root;
        morphFn(el, html);
      } else {
        morphFn(root, html);
      }
    }
    const opFn = extras.applyOp ?? applyOp;
    const doc = extras.document ?? root.ownerDocument;
    for (const op of msg.ops || []) opFn(op, doc, extras);
    if (msg.patch) extras.history?.pushState?.({}, "", msg.patch);
    if (msg.navigate && extras.location) extras.location.href = msg.navigate;
    const pushFn = extras.dispatchPush ?? dispatchLivePush;
    for (const p of msg.pushes || []) pushFn(p.event, p.payload);
    if (doc && typeof doc.dispatchEvent === "function") {
      doc.dispatchEvent(new CustomEvent("amarra:morphed", { bubbles: true }));
    }
  }
  function cssEscape(id) {
    if (typeof CSS !== "undefined" && CSS.escape) return CSS.escape(id);
    return String(id).replace(/[^a-zA-Z0-9_-]/g, "\\$&");
  }
  function start4(opts = {}) {
    const doc = opts.document ?? (typeof document !== "undefined" ? document : null);
    if (!doc || typeof doc.addEventListener !== "function") return;
    if (doc.documentElement?.dataset?.amarraLive === "true") return;
    if (doc.documentElement?.dataset) doc.documentElement.dataset.amarraLive = "true";
    const WS = opts.WebSocket ?? (typeof WebSocket !== "undefined" ? WebSocket : null);
    const location = opts.location ?? (typeof window !== "undefined" ? window.location : null);
    const sockets = /* @__PURE__ */ new Map();
    function connect2(root) {
      if (!root || !WS || !location) return;
      const view = root.getAttribute("amarra-live");
      if (!view) return;
      const topic = root.getAttribute("data-amarra-topic") || view;
      const url = wsURL(location, { view, topic });
      const ws = new WS(url);
      sockets.set(root, ws);
      ws.addEventListener("open", () => {
        const csrf = opts.csrfToken ?? csrfTokenFromMeta(doc);
        ws.send(JSON.stringify({ type: "join", csrf }));
      });
      ws.addEventListener("message", (ev) => {
        let msg;
        try {
          msg = JSON.parse(ev.data);
        } catch {
          return;
        }
        applyLiveMessage(msg, root, opts.morphFn, {
          document: doc,
          history: opts.history ?? (typeof window !== "undefined" ? window.history : null),
          location
        });
        setLoading(root._amarraPending, root, false);
        root._amarraPending = null;
      });
      ws.addEventListener("close", () => {
        sockets.delete(root);
        const wait = opts.reconnectMs ?? 1e3;
        if (wait < 0) return;
        setTimeout(() => {
          if (!doc.contains?.(root) && root.isConnected === false) return;
          connect2(root);
        }, wait);
      });
    }
    function sync() {
      for (const [root, ws] of sockets) {
        if (root.isConnected === false) {
          ws?.close?.();
          sockets.delete(root);
        }
      }
      doc.querySelectorAll?.("[amarra-live]").forEach((el) => {
        if (!sockets.has(el)) connect2(el);
      });
    }
    sync();
    if (typeof doc.addEventListener === "function") doc.addEventListener("amarra:morphed", sync);
    function sendFrom(el, kind, extra) {
      const root = liveRoot(el);
      if (!root) return false;
      const name = eventName(kind === "submit" ? el : el.closest?.(`[amarra-${kind}]`) || el, kind);
      if (!name) return false;
      const ws = sockets.get(root);
      if (!ws || ws.readyState !== 1) return false;
      const payload = extra ?? {};
      const send = () => {
        setLoading(el, root, true);
        root._amarraPending = el;
        ws.send(JSON.stringify({ type: "event", event: name, payload, ref: String(Date.now()) }));
      };
      const wait = debounceWait(el);
      if (wait) {
        clearTimeout(el._amarraDebounce);
        el._amarraDebounce = setTimeout(send, wait);
        return true;
      }
      send();
      return true;
    }
    doc.addEventListener(
      "click",
      (ev) => {
        const btn = ev.target?.closest?.("[amarra-click]");
        if (!btn || !liveRoot(btn)) return;
        ev.preventDefault();
        sendFrom(btn, "click", { value: btn.value ?? btn.textContent ?? "" });
      },
      true
    );
    doc.addEventListener(
      "change",
      (ev) => {
        const el = ev.target?.closest?.("[amarra-change]");
        if (!el || !liveRoot(el)) return;
        sendFrom(el, "change", { value: el.value ?? "" });
      },
      true
    );
    doc.addEventListener(
      "submit",
      (ev) => {
        const form = ev.target?.closest?.("form[amarra-submit]");
        if (!form || !liveRoot(form)) return;
        ev.preventDefault();
        sendFrom(form, "submit", formPayload(form, opts.FormData));
      },
      true
    );
  }

  // pkg/amarra/js/entry.mjs
  if (typeof globalThis !== "undefined") {
    globalThis.Idiomorph = Idiomorph;
  }
  function boot() {
    if (typeof window === "undefined") return;
    start();
    start3();
    define();
    start2();
    start4();
    window.amarra = {
      drive: drive_exports,
      live: live_exports,
      hook: hook_exports
    };
  }
  if (typeof window !== "undefined") {
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", boot);
    } else {
      boot();
    }
  }
})();
