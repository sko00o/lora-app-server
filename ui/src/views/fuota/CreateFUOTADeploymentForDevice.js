import React, { Component } from "react";
import { withRouter, Link } from 'react-router-dom';

import { withStyles } from "@material-ui/core/styles";
import Grid from '@material-ui/core/Grid';
import Button from "@material-ui/core/Button";

import TitleBar from "../../components/TitleBar";
import TitleBarTitle from "../../components/TitleBarTitle";

import FUOTADeploymentStore from "../../stores/FUOTADeploymentStore";


const styles = {};


class CreateFUOTADeploymentForDevice extends Component {
  constructor() {
    super();
    this.onSubmit = this.onSubmit.bind(this);
  }

  onSubmit(fuotaDeployment) {
    FUOTADeploymentStore.createForDevice(this.props.match.params.devEUI, fuotaDeployment, resp => {
      this.props.history.push(`/organizations/${this.props.match.params.organizationID}/fuota-deployments/${resp.id}`);
    });
  }

  render() {
    return(
      <Grid container spacing={24}>
        <TitleBar>

        </TitleBar>
      </Grid>
    );
  }
}

export default withStyles(styles)(withRouter(CreateFUOTADeploymentForDevice));

