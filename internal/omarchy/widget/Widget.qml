import QtQuick
import Quickshell
import qs.Commons
import qs.Ui

// Agent Link's icon in Omarchy's bar, put there by Agent Link (Settings → Bar icon).
// A click drops down its panel under the bar, where the click was; a right
// click opens its window. Agent Link is started if it isn't running.
BarWidget {
  id: root
  moduleName: "useagentlink.agentlink"

  // written in by Agent Link when it puts the widget here
  readonly property string agentlink: "__AGENT_LINK__"

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: String.fromCodePoint(0xf15c6) // nf-md-bird
    tooltipText: "Agent Link"
    onPressed: function(b) {
      Quickshell.execDetached(b === Qt.RightButton ? [root.agentlink, "app"] : [root.agentlink, "panel"])
    }
  }
}
