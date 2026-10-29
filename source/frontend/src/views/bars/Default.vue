<template>
  <header v-if="!isOnlyOffice" :class="['flexbar', { 'dark-mode-header': isDarkMode }]">
    <action
      v-if="!disableNavButtons"
      class="nav-toggle-button"
      icon="close_back"
      :label="$t('general.close')"
      :disabled="isDisabledMultiAction"
      @action="multiAction"
    />
    <div v-if="courseName" class="header-course">
      <a class="header-back" href="/" title="返回所有作业列表">
        <i class="material-symbols">arrow_back</i><span class="header-back-text">返回所有作业列表</span>
      </a>
      <span class="header-course-name">{{ courseName }}</span>
    </div>
    <div class="search-bar-container" :class="{ disabled: isDisabled }" 
         v-if="showSearch && !isSearchActive" @click="openSearch" >
      <i class="material-symbols">search</i>
      <input 
        type="text" 
        id="search-bar-input" 
        :placeholder="$t('general.search', { suffix: '...' })" 
        readonly
      />
    </div>
    <title v-if="!showSearch" class="topTitle">{{ getTopTitle }}</title>
    <action
      v-if="showHeaderSwitchView && !disableNavButtons"
      class="menu-button"
      :icon="viewIcon"
      :label="viewModeActionLabel"
      @action="switchView"
      :disabled="isDisabled || viewModeChangeLocked"
    />
    <action
      class="overflow-menu-button"
      v-else-if="!showHeaderSwitchView && !showQuickSave"
      :icon="iconName"
      :disabled="noItems"
      @click="toggleOverflow"
    />
    <action
      class="save-button"
      v-else-if="showQuickSave"
      id="save-button"
      icon="save"
      :label="$t('general.save')"
      @action="save()"
    />
  </header>
</template>

<script>
import router from "@/router";
import buttons from "@/utils/buttons";
import { notify } from "@/notify";
import { getters, state, mutations } from "@/store";
import Action from "@/components/Action.vue";
import { globalVars } from "@/utils/constants";
import { url } from "@/utils";
import { COURSE_NAME } from "@/utils/submitOnly.js";

export default {
  name: "UnifiedHeader",
  components: {
    Action,
  },
  data() {
    return {
      viewModes: ["list", "normal", "icons"],
    };
  },
  computed: {
    courseName: () => COURSE_NAME,
    getTopTitle() {
      if (getters.isSettings()) {
        return this.$t("general.settings");
      }
      if (getters.isShare()) {
        if (state.req?.type === "directory" || state.shareInfo?.shareType === "upload") {
          return state.shareInfo?.title;
        }
        return state.req.name;
      }
      const currentTool = getters.currentTool();
      if (currentTool) {
        return currentTool.name;
      }
      return state.req.name;
    },
    showQuickSave() {
      if (getters.currentView() !== "editor" || !getters.sourcePermissions().modify) {
        return false;
      }
      return state.user.editorQuickSave;
    },
    disableNavButtons() {
      const isShare = getters.isShare();
      const regularDisabled = globalVars.disableNavButtons && this.isListingView;
      const shareDisabled = isShare && state.shareInfo?.hideNavButtons && getters.currentView() === "listingView";
      const uploadShare = isShare && state.shareInfo?.shareType === "upload"
      return regularDisabled || shareDisabled || uploadShare;
    },
    isOnlyOffice() {
      return getters.currentView() === "onlyOfficeEditor";
    },
    isListingView() {
      return getters.currentView() === "listingView";
    },
    isAdvancedSearchRoute() {
      return (state.route?.path || "") === "/tools/advancedSearch";
    },
    showHeaderSwitchView() {
      return this.isListingView || this.isAdvancedSearchRoute;
    },
    iconName() {
      return getters.currentPromptName() === "OverflowMenu"
        ? "keyboard_arrow_up"
        : "more_vert";
    },
    viewIcon() {
      const icons = {
        list: "view_list",
        compact: "view_list",
        normal: "view_module",
        gallery: "grid_view",
      };
      return icons[getters.viewMode()] || "grid_view";
    },
    isShare() {
      return getters.isShare();
    },
    noItems() {
      return !state.contextMenuHasItems && !getters.isPreviewView();
    },
    showEdit() {
      return window.location.hash !== "#edit" && getters.sourcePermissions().modify;
    },
    showDelete() {
      return getters.sourcePermissions().delete && getters.currentView() === "preview";
    },
    showSave() {
      return getters.currentView() === "editor" && getters.sourcePermissions().modify;
    },
    showSearch() {
      return getters.isLoggedIn() && getters.currentView() === "listingView" && !getters.isShare();
    },
    isSearchActive() {
      return state.isSearchActive;
    },
    isDisabled() {
      return state.isSearchActive || getters.currentPromptName() !== "";
    },
    viewModeChangeLocked() {
      return getters.viewModeChangeLocked();
    },
    viewModeActionLabel() {
      if (this.viewModeChangeLocked) {
        return this.$t("profileSettings.enforcedByAdmin");
      }
      return this.$t("buttons.switchView");
    },
    isDisabledMultiAction() {
      const regularDisabled = getters.isStickySidebar() && getters.multibuttonState() === "menu";
      const shareDisabled = state.shareInfo?.disableSidebar && getters.multibuttonState() === "menu";
      return this.isDisabled || regularDisabled || shareDisabled;
    },
    showSwitchView() {
      return this.showHeaderSwitchView;
    },
    showSidebarToggle() {
      return getters.currentView() === "listingView";
    },
    req() {
      return state.req;
    },
    isDarkMode() {
      return getters.isDarkMode();
    },
    isSettings() {
      return getters.isSettings();
    },
  },
  methods: {
    openSearch() {
      if (!state.isSearchActive && !this.isDisabled) {
        mutations.closeHovers();
        mutations.closeSidebar();
        mutations.resetSelected();
        mutations.setSearch(true);
        // this is hear to allow for animation
        setTimeout(() => {
          const resultList = document.getElementById("result-list");
          resultList.classList.add("active");
          document.getElementById("search-input").focus();
        }, 100);
      }
    },
    async save() {
      const button = "save";
      buttons.loading("save");
      try {
        // Call the editor's save handler directly
        if (state.editorSaveHandler) {
          await state.editorSaveHandler();
          buttons.success(button);
          // Note: Success notification is shown by the editor
        } else {
          const errorMsg = "No editor save handler registered";
          notify.showError(errorMsg);
          throw new Error(errorMsg);
        }
      } catch (e) {
        buttons.done(button);
        // Note: Error notification is already shown by the editor
        throw e; // Re-throw so caller knows save failed
      }
    },
    toggleOverflow() {
      if (getters.currentPromptName() === "OverflowMenu") {
        mutations.closeHovers();
      } else {
        mutations.showPrompt({ name: "OverflowMenu" });
      }
    },
    /** Match StatusBar.adjustViewMode: list vs compact, icons vs gallery from gallery size. */
    resolveViewModeForFamily(baseMode) {
      const size = state.user?.gallerySize ?? 5;
      if (baseMode === "list") {
        return size <= 3 ? "compact" : "list";
      }
      if (baseMode === "icons") {
        return size <= 4 ? "icons" : "gallery";
      }
      return baseMode;
    },
    /** Map concrete viewMode to one of the three switch-view families (list|compact → list, etc.). */
    viewModeCycleIndex(mode) {
      if (mode === "list" || mode === "compact") {
        return 0;
      }
      if (mode === "normal") {
        return 1;
      }
      if (mode === "gallery" || mode === "icons") {
        return 2;
      }
      return 1;
    },
    switchView() {
      mutations.closeHovers();
      const current = getters.viewMode();
      const cycleIndex = this.viewModeCycleIndex(current);
      const nextIndex = (cycleIndex + 1) % this.viewModes.length;
      const baseMode = this.viewModes.at(nextIndex);
      const newViewMode = this.resolveViewModeForFamily(baseMode);
      mutations.updateDisplayPreferences({ viewMode: newViewMode });
      void mutations.updateCurrentUser({ viewMode: newViewMode });
    },
    multiAction() {
      const cv = getters.currentView();

      // Check for unsaved editor changes before navigation
      if (cv === "editor" && state.editorDirty) {
        this.showSaveBeforeExitPrompt(() => this.performNavigation(cv));
        return;
      }

      this.performNavigation(cv);
    },
    performNavigation(cv) {
      if (cv === "listingView" || ( getters.isShare() && getters.multibuttonState() !== "close") || cv === "tools") {
        mutations.toggleSidebar();
      } else if (cv === "settings" && getters.isMobile()) {
        mutations.toggleSidebar();
      } else {
        mutations.closeHovers();
        if (cv === "settings") {
          if (state.previousHistoryItem?.name) {
            url.goToItem(
              state.previousHistoryItem.source,
              state.previousHistoryItem.path,
              state.previousHistoryItem,
              false,
              state.previousHistoryItem.isShare
            );
            return;
          }
          if (state.shareInfo?.hash && state.req?.source === state.shareInfo.hash) {
            url.goToItem(state.shareInfo.hash, state.req.path, {}, false, true);
            return;
          }
          // otherwise navigate to files
          void router.push({ path: "/files" });
          return;
        }
        if (getters.isPreviewView()) {
          if (state.previousHistoryItem?.name) {
            url.goToItem(state.previousHistoryItem.source, state.previousHistoryItem.path, state.previousHistoryItem, false, state.previousHistoryItem.isShare);
            return;
          } else {
            // navigate to parent directory of current url
            const parentPath = url.removeLastDir(state.route.path);
            void router.push({ path: parentPath });
          }
          return;
        }

        router.go(-1);
      }
    },
    showSaveBeforeExitPrompt(onConfirmAction) {
      mutations.showPrompt({
        name: "SaveBeforeExit",
        pinned: true,
        confirm: async () => {
          // Save and exit - trigger the save action
          // If save fails, this will throw and be caught by SaveBeforeExit component
          await this.save();
          mutations.setEditorDirty(false);
          onConfirmAction();
        },
        discard: () => {
          // Discard changes and exit
          mutations.setEditorDirty(false);
          onConfirmAction();
        },
        cancel: () => {
          // Keep editing - do nothing
        },
      });
    },
  },
};
</script>

<style scoped>
/* HW-SUBMIT: back link + course name, kept together at the left */
.header-course {
  flex: 1 1 0; /* take the free space on the left so the group sits right after the menu button */
  min-width: 0;
  display: flex;
  align-items: center;
  margin: 0 1em 0 0.25em;
}

.header-back {
  display: flex;
  align-items: center;
  gap: 0.25em;
  flex: none;
  padding: 0.35em 0.6em 0.35em 0.4em;
  border-radius: 0.8em;
  color: var(--primaryColor);
  font-size: 0.95em;
  text-decoration: none;
  white-space: nowrap;
}

.header-back:hover {
  background: var(--surfaceSecondary);
}

.header-back .material-symbols {
  font-size: 1.3em;
}

.header-course-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
  font-size: 1.1em;
  color: var(--textPrimary);
  padding-left: 0.75em;
  margin-left: 0.25em;
  border-left: 1px solid var(--divider);
}

@media (max-width: 800px) {
  .header-back-text {
    display: none;
  }
  .header-back {
    padding-right: 0.4em;
  }
  .header-course {
    flex: 0 1 auto;
    margin-right: 0.5em;
  }
}

header button:hover {
  box-shadow: unset !important;
  -webkit-box-shadow: unset !important;
}
header {
  background-color: color-mix(in srgb, var(--alt-background) 15%, transparent);
}
/* Header with backdrop-filter support */
@supports (backdrop-filter: none) {
  header {
    backdrop-filter: blur(16px) invert(0.1);
  }
}

:deep(.action.nav-toggle-button .line),
:deep(.action.menu-button),
:deep(.action.overflow-menu-button) {
  stroke: #546e7a;
  color: #546e7a;
}


.search-bar-container {
  display: flex;
  align-items: center;
  background-color: rgba(100, 100, 100, 0.2);
  border-radius: 1em;
  padding: 0.5em 0.75em;
  transition: background-color 0.2s ease;
  gap: 0.5em;
  min-width: 35em;
  max-width: 300px;
  flex: 1;
  height: 3em;
  box-sizing: border-box;
}

/* prevent open search if a prompt is open */
.search-bar-container.disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.search-bar-container.disabled #search-bar-input {
  pointer-events: none;
}

@media (max-width: 768px) {
  .search-bar-container {
    min-width: unset;
    max-width: 60%;
  }
}

.search-bar-container:hover {
  background-color: rgba(100, 100, 100, 0.3);
}

.search-bar-container .material-symbols {
  font-size: 1.25em;
  user-select: none;
}

#search-bar-input {
  background: transparent;
  border: none;
  outline: none;
  color: rgba(255, 255, 255, 0.9);
  width: 100%;
  font-size: 0.95em;
  user-select: none;
}

#search-bar-input::placeholder {
  color: gray;
}


.dark-mode-header .search-bar-container {
  background-color: rgba(100, 100, 100, 0.2);
}

.dark-mode-header .search-bar-container:hover {
  background-color: rgba(255, 255, 255, 0.15);
}
</style>
